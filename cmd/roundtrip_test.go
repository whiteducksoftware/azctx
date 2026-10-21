package cmd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		args []string
		code int
		text string
	}{
		{args: []string{"-c"}, text: "Active subscription:"},
		{args: []string{loginCommand, argumentDelimiter, deviceCodeFlag}, text: "Successfully fetched 0 tenants and 2 subscriptions."},
		{args: []string{loginCommand}, code: 1, text: "failed to login: denied"},
		{args: []string{productionName}, code: 1, text: "Error: unknown command \"Production\" for \"azctx\"\nRun 'azctx --help' for usage.\n"},
		{args: []string{"-"}, code: 1, text: "not implemented"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			command := exec.Command(binary, tc.args...) //nolint:gosec // Test-owned binary and fixed fixture arguments, without a shell.
			output, err := command.CombinedOutput()
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				code = exitErr.ExitCode()
			}
			if code != tc.code || !strings.Contains(string(output), tc.text) {
				t.Fatalf("exit=%d output=%s; want exit=%d containing %q", code, output, tc.code, tc.text)
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
