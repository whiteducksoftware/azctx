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
	config := filepath.Join(dir, "config")
	if err := os.Mkdir(config, 0o700); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(config, "azureProfile.json")
	writeFixture(t, profilePath, fixtureProfile)
	fixtures, record := filepath.Join(dir, "fixtures.json"), filepath.Join(dir, "calls.jsonl")
	writeFixture(t, fixtures, `[
		{"args":["rest","--method","get","--url","/tenants?api-version=2020-01-01","--output","json"],"stdout":"{\"value\":[]}"},
		{"args":["login","--use-device-code"]},
		{"args":["rest","--method","get","--url","/tenants?api-version=2020-01-01","--output","json"],"stdout":"{\"value\":[]}"},
		{"args":["login"],"stderr":"ERROR: denied\n","exitCode":7}
	]`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZURE_CONFIG_DIR", config)
	t.Setenv("FAKEAZ_FIXTURES", fixtures)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("FAKEAZ_ENV_KEYS", "AZURE_CONFIG_DIR")
	for _, tc := range []struct {
		args  []string
		code  int
		text  string
		input string
		data  bool
	}{
		{args: []string{"-c"}, text: "Active subscription:", data: true},
		{args: []string{versionCommand}, text: "azctx version ", data: true},
		{args: []string{versionCommand, outputFlag, jsonFormat}, text: `"goVersion":"go`, data: true},
		{args: []string{loginCommand, argumentDelimiter, deviceCodeFlag}, text: "Successfully fetched 0 tenants and 2 subscriptions."},
		{args: []string{loginCommand}, code: 1, text: "failed to login: denied"},
		{args: []string{productionName}, code: 1, text: "Error: unknown command \"Production\" for \"azctx\"\nRun 'azctx --help' for usage.\n"},
		{args: []string{"-"}, code: 1, text: "not implemented"},
		{input: "\x03", code: 130},
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
	got, err := os.ReadFile(profilePath)
	if err != nil || string(got) != fixtureProfile {
		t.Fatalf("profile changed: %q, %v", got, err)
	}
	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 4 {
		t.Fatalf("recorded calls = %s, %v", calls, err)
	}
}

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
