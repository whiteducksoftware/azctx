package azcli_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestCLIChildConfigDir(t *testing.T) {
	t.Setenv("AZURE_CLOUD_NAME", publicCloud)
	inherited := testutil.WorkDir(t)
	t.Setenv("AZURE_CONFIG_DIR", inherited)
	const subscription = "11111111-1111-1111-1111-111111111111"
	const tenant = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	data := []byte(`{"subscriptions":[{"id":"` + subscription + `","tenantId":"` + tenant + `","environmentName":"AzureCloud"}]}`)
	if err := os.WriteFile(filepath.Join(inherited, profileFilename), data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, constructor := range []struct {
		name string
		new  func(fs.FS, azcli.Runner, string) (azcli.CLI, error)
	}{
		{"profile", azcli.New},
		{loginCommand, azcli.NewForLogin},
	} {
		for _, configDir := range []string{"", filepath.Join("synthetic", "explicit azure")} {
			for _, injectedFS := range []bool{true, false} {
				if configDir != "" && !injectedFS {
					continue
				}
				t.Run(constructor.name+"/"+configDir+"/"+map[bool]string{true: "injected", false: "resolved"}[injectedFS], func(t *testing.T) {
					var files fs.FS
					if injectedFS {
						files = fstest.MapFS{profileFilename: &fstest.MapFile{Data: data}}
					}
					fake := &testutil.FakeRunner{Responses: []testutil.Response{
						{},
						{Result: azcli.Result{Stdout: mfaMinimumVersionJSON}},
						{},
						{},
						{Result: azcli.Result{Stdout: `[]`}},
					}}
					cli, err := constructor.new(files, fake, configDir)
					if err != nil {
						t.Fatal(err)
					}
					cli.Terminal = &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}
					if err := cli.SetSubscription(cli.Subscriptions()[0]); err != nil {
						t.Fatal(err)
					}
					if _, err := cli.Login(nil); err != nil {
						t.Fatal(err)
					}
					if _, err := cli.LoginTenant(tenant, nil); err != nil {
						t.Fatal(err)
					}
					if _, err := cli.DiscoverTenants(); err != nil {
						t.Fatal(err)
					}
					assertChildConfigDir(t, fake.Calls, configDir)
					if got := os.Getenv("AZURE_CONFIG_DIR"); got != inherited {
						t.Fatalf("parent config directory changed: %q", got)
					}
				})
			}
		}
	}
}

func assertChildConfigDir(t *testing.T, calls []testutil.Call, configDir string) {
	t.Helper()
	if len(calls) != 5 {
		t.Fatalf("expected switch/version/login/tenant login/discovery, got %+v", calls)
	}
	for _, call := range calls {
		var overrides []string
		for _, entry := range call.Env {
			if strings.HasPrefix(entry, "AZURE_CONFIG_DIR=") {
				overrides = append(overrides, entry)
			}
		}
		var want []string
		if configDir != "" {
			want = []string{"AZURE_CONFIG_DIR=" + configDir}
		}
		if !slices.Equal(overrides, want) {
			t.Errorf("az %q config overrides = %q, want %q", call.Args, overrides, want)
		}
		if call.Args[0] == loginCommand && (!call.Attached || !slices.Contains(call.Env, "AZURE_CORE_LOGIN_EXPERIENCE_V2=off")) {
			t.Errorf("login child settings lost: %+v", call)
		}
	}
}
