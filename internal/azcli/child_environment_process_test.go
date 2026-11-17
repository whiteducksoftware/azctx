package azcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	childVersionCommand = "version"
	childOutputFlag     = "-o"
	childTenantFlag     = "--tenant"
	childPickerOffValue = "off"
)

func TestCLIChildEnvironmentFakeAZ(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, configDir := range []string{"", filepath.Join("synthetic", "explicit azure")} {
		t.Run(configDir, func(t *testing.T) {
			const tenant = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			t.Setenv("AZURE_CONFIG_DIR", "inherited azure")
			t.Setenv("AZURE_CORE_LOGIN_EXPERIENCE_V2", "on")
			t.Setenv("AZURE_EXTENSION_USE_DYNAMIC_INSTALL", "yes_without_prompt")
			fixtures := []loginFixture{
				{Args: []string{childVersionCommand, childOutputFlag, jsonOutput}, Stdout: mfaMinimumVersionJSON},
				{Args: []string{loginCommand}, Stderr: "WARNING: Authentication failed against tenant " + tenant + " 'Token': AADSTS50076: token=synthetic-sensitive-value\n"},
				{Args: []string{accountCommand, "tenant", "list", childOutputFlag, jsonOutput}, Stdout: `[{"tenantId":"` + tenant + `"}]`},
				{Args: []string{loginCommand, childTenantFlag, tenant}},
			}
			runner := fakeLoginProcess(t, dir, fixtures)
			record := filepath.Join(testutil.WorkDir(t), "calls.jsonl")
			t.Setenv("FAKEAZ_RECORD", record)
			t.Setenv("FAKEAZ_ENV_KEYS", "AZURE_CONFIG_DIR,AZURE_CORE_LOGIN_EXPERIENCE_V2,AZURE_EXTENSION_USE_DYNAMIC_INSTALL")
			cli, err := azcli.NewForLogin(fstest.MapFS{}, runner, configDir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cli.Context = ctx
			var stderr bytes.Buffer
			cli.Terminal = &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr}
			result, err := cli.Login(nil)
			if err != nil || len(result.Failures) != 1 || result.Failures[0].ID != tenant || result.Failures[0].Code != "50076" {
				t.Fatalf("typed tenant observation lost: result=%+v error=%v", result, err)
			}
			assertRedactedLoginDiagnostics(t, "synthetic-sensitive-value", result.Failures[0].Message, stderr.String(), runner.results[1].Stderr)
			tenants, err := cli.DiscoverTenants()
			if err != nil || !slices.Equal(tenants, []string{tenant}) {
				t.Fatalf("tenants=%q error=%v", tenants, err)
			}
			if _, err := cli.LoginTenant(tenant, nil); err != nil {
				t.Fatal(err)
			}
			assertFakeAZChildEnvironment(t, record, configDir)
		})
	}
}

func assertFakeAZChildEnvironment(t *testing.T, record, configDir string) {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected version/login/discovery/tenant login: %s", data)
	}
	if configDir == "" {
		configDir = "inherited azure"
	}
	for i, line := range lines {
		var call testutil.Call
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatal(err)
		}
		picker, dynamic := "on", "yes_without_prompt"
		if i == 1 || i == 3 {
			picker = childPickerOffValue
		}
		if i == 2 {
			dynamic = "no"
		}
		want := []string{
			"AZURE_CONFIG_DIR=" + configDir,
			"AZURE_CORE_LOGIN_EXPERIENCE_V2=" + picker,
			"AZURE_EXTENSION_USE_DYNAMIC_INSTALL=" + dynamic,
		}
		if !slices.Equal(call.Env, want) {
			t.Errorf("az %q child env=%q, want %q", call.Args, call.Env, want)
		}
	}
	for key, want := range map[string]string{
		"AZURE_CONFIG_DIR":                    "inherited azure",
		"AZURE_CORE_LOGIN_EXPERIENCE_V2":      "on",
		"AZURE_EXTENSION_USE_DYNAMIC_INSTALL": "yes_without_prompt",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("parent %s=%q, want %q", key, got, want)
		}
	}
}
