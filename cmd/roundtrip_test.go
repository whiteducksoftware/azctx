package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBinaryFakeAZRoundTrip(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	binary := filepath.Join(dir, testutil.ExecutableName("azctx"))
	testutil.Build(t, binary, "github.com/whiteducksoftware/azctx")
	testutil.IsolateUserDirs(t)
	config := filepath.Join(dir, "config")
	if err := os.Mkdir(config, 0o700); err != nil {
		t.Fatal(err)
	}
	azureFiles := map[string]string{
		profileFile:         fixtureProfile,
		azureConfigFile:     "[cloud]\nname=AzureCloud\n[core]\nlogin_experience_v2=on\n",
		cloudsConfigFile:    "[AzureCloud]\nsubscription=" + productionID + "\n",
		"azctxTenants.json": "malformed obsolete-cache sentinel",
	}
	for name, contents := range azureFiles {
		writeFixture(t, filepath.Join(config, name), contents)
	}
	fixtures, record := filepath.Join(dir, "fixtures.json"), filepath.Join(dir, "calls.jsonl")
	writeFixture(t, fixtures, `[
		{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
		{"args":["login","--use-device-code"]},
		{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
		{"args":["login"],"stderr":"ERROR: denied\n","exitCode":7},
		{"args":["account","set","--subscription","`+productionID+`"]}
	]`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZURE_CONFIG_DIR", config)
	t.Setenv("FAKEAZ_FIXTURES", fixtures)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("AZURE_CLOUD_NAME", azureCloudName)
	t.Setenv("AZURE_CORE_LOGIN_EXPERIENCE_V2", "on")
	t.Setenv("FAKEAZ_ENV_KEYS", "AZURE_CONFIG_DIR,AZURE_CORE_LOGIN_EXPERIENCE_V2")
	// An interruption regression must fail locally, never contact GitHub.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:0")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:0")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	for _, tc := range []struct {
		args  []string
		code  int
		text  string
		input string
		data  bool
	}{
		{args: []string{currentFlag}, text: "Active subscription:", data: true},
		{args: []string{versionCommand}, text: "azctx version ", data: true},
		{args: []string{versionCommand, outputFlag, jsonFormat}, text: `"goVersion":"go`, data: true},
		{args: []string{loginCommand, argumentDelimiter, deviceCodeFlag}, text: "Successfully fetched 1 tenants and 2 subscriptions."},
		{args: []string{loginCommand}, code: 3, text: "Azure CLI sign-in failed; see its attached output"},
		{args: []string{productionName}},
		{args: []string{"-"}, code: 1, text: "no previous subscription found"},
		// A Ctrl+C byte in a pipe is data, not an interactive cancellation.
		{input: "\x03", text: "NAME", data: true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, tc.args...) //nolint:gosec // Test-owned binary and fixed fixture arguments, without a shell.
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			command.Stdin = strings.NewReader(tc.input)
			err := command.Run()
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				code = exitErr.ExitCode()
			}
			output := stderr.String()
			if tc.data {
				output = stdout.String()
			}
			if code != tc.code || !strings.Contains(output, tc.text) {
				t.Fatalf("exit=%d stdout=%q stderr=%q; want exit=%d containing %q", code, stdout.String(), stderr.String(), tc.code, tc.text)
			}
			if !tc.data && stdout.Len() != 0 {
				t.Fatalf("diagnostics leaked to stdout: %q", stdout.String())
			}
			if bytes.ContainsRune(stderr.Bytes(), '\a') {
				t.Fatal("terminal bell emitted")
			}
		})
	}
	assertAzureRoundTrip(t, config, record, azureFiles)
}

func assertAzureRoundTrip(t *testing.T, config, record string, azureFiles map[string]string) {
	t.Helper()
	for name, want := range azureFiles {
		got, err := os.ReadFile(filepath.Join(config, name))
		if err != nil || string(got) != want {
			t.Fatalf("Azure file %s changed: %q, %v", name, got, err)
		}
	}
	entries, err := os.ReadDir(config)
	if err != nil || len(entries) != len(azureFiles) {
		t.Fatalf("Azure files created/deleted: %+v, %v", entries, err)
	}
	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 5 ||
		strings.Count(string(calls), `"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"`) != 2 ||
		strings.Count(string(calls), `"AZURE_CORE_LOGIN_EXPERIENCE_V2=on"`) != 3 {
		t.Fatalf("recorded calls = %s, %v", calls, err)
	}
	if got := os.Getenv("AZURE_CORE_LOGIN_EXPERIENCE_V2"); got != "on" {
		t.Fatalf("parent environment changed: %q", got)
	}
}

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
