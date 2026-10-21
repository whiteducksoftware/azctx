package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	envCommand            = "env"
	shellFlag             = "--shell"
	formatFlag            = "--format"
	envBashSyntax         = "bash"
	envZshSyntax          = "zsh"
	envFishSyntax         = "fish"
	envPSSyntax           = "powershell"
	envGitHubFormat       = "github"
	envDevOpsFormat       = "azure-devops"
	envPipelineFile       = "github.env"
	exportAliasName       = "prod"
	envConfigFilename     = "config.yaml"
	directoryScenario     = "directory"
	subscriptionInjection = "subscription injection"
)

func TestEnvironmentBashIsReadOnly(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	deps.Runner = nil // Export must not even require an installed Azure CLI.
	configPath := writeSelectionConfig(t, "aliases:\n  prod: "+productionID+"\nprotected: ["+productionID+"]\n")
	var out, messages bytes.Buffer
	code := cmd.Run([]string{envCommand, exportAliasName, shellFlag, envBashSyntax}, deps, strings.NewReader(""), &out, &messages)
	want := "export ARM_SUBSCRIPTION_ID='" + productionID + "'\n" +
		"export ARM_TENANT_ID='" + tenantID + "'\n" +
		"export AZURE_SUBSCRIPTION_ID='" + productionID + "'\n" +
		"export AZURE_TENANT_ID='" + tenantID + "'\n"
	if code != 0 || out.String() != want || messages.Len() != 0 || len(fake.Calls) != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
	}
	entries, err := os.ReadDir(filepath.Dir(configPath))
	if err != nil || len(entries) != 1 || entries[0].Name() != envConfigFilename {
		t.Fatalf("unexpected export mutation: %v, %v", entries, err)
	}
}

func TestEnvironmentFormatGoldens(t *testing.T) {
	for _, format := range []string{envBashSyntax, envZshSyntax, envFishSyntax, envPSSyntax, envGitHubFormat, envDevOpsFormat} {
		t.Run(format, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Runner = nil
			path := filepath.Join(testutil.WorkDir(t), envPipelineFile)
			if err := os.WriteFile(path, []byte("EXISTING=value\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GITHUB_ENV", path)
			args := []string{envCommand}
			if format == envGitHubFormat || format == envDevOpsFormat {
				args = append(args, formatFlag, format)
			} else {
				args = append(args, shellFlag, format)
			}
			var out, messages bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
			if code != 0 || messages.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
			}
			got := out.Bytes()
			if format == envGitHubFormat {
				if len(got) != 0 {
					t.Fatalf("github format printed stdout: %q", got)
				}
				var err error
				got, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(filepath.Join("testdata", "env", format+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) || bytes.ContainsRune(got, '\r') || !bytes.HasSuffix(got, []byte("\n")) {
				t.Fatalf("format %s\ngot: %q\nwant: %q", format, got, want)
			}
		})
	}
}

func TestEnvironmentRejectsUnsafeInputAndDestinations(t *testing.T) {
	for _, scenario := range []string{missingQuery, "absent file", directoryScenario, "relative", "azure", "azure symlink", "azure parent symlink", "azure hardlink", "readonly", "unknown format", "unknown shell", "conflicting formats", subscriptionInjection, "tenant injection"} {
		t.Run(scenario, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			args, protected := unsafeEnvironmentArgs(t, &deps, scenario)
			var out, messages bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
			if code != 1 || out.Len() != 0 || messages.Len() == 0 || len(fake.Calls) != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
			}

			data, err := os.ReadFile(protected)
			if err != nil || string(data) != "unchanged\n" {
				t.Fatalf("Azure config mutated: %q, %v", data, err)
			}
		})
	}
}

func unsafeEnvironmentArgs(t *testing.T, deps *cmd.Dependencies, scenario string) ([]string, string) {
	t.Helper()
	dir := testutil.WorkDir(t)
	azure := filepath.Join(dir, ".azure")
	if err := os.Mkdir(azure, 0o700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(azure, "config")
	writeFixture(t, protected, "unchanged\n")
	t.Setenv("AZURE_CONFIG_DIR", azure)
	t.Setenv("GITHUB_ENV", unsafeEnvironmentDestination(t, scenario, dir, azure, protected))
	args := []string{envCommand, formatFlag, envGitHubFormat}
	switch scenario {
	case "unknown format":
		args = []string{envCommand, formatFlag, "other"}
	case "unknown shell":
		args = []string{envCommand, shellFlag, "other"}
	case "conflicting formats":
		args = append(args, shellFlag, envBashSyntax)
	case subscriptionInjection, "tenant injection":
		args = []string{envCommand}
		data := fixtureProfile
		if scenario == subscriptionInjection {
			data = strings.ReplaceAll(data, productionID, `x'; touch nope; #\nINJECTED=bad`)
		} else {
			data = strings.ReplaceAll(data, tenantID, `x'\nINJECTED=bad`)
		}
		deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(data)}}
	}
	return args, protected
}

func unsafeEnvironmentDestination(t *testing.T, scenario, dir, azure, protected string) string {
	t.Helper()
	path := filepath.Join(dir, envPipelineFile)
	switch scenario {
	case missingQuery:
		return ""
	case directoryScenario:
		return dir
	case "relative":
		return envPipelineFile
	case "azure":
		return protected
	case "azure symlink":
		if err := os.Symlink(protected, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	case "azure parent symlink":
		link := filepath.Join(dir, "linked-dir")
		if err := os.Symlink(azure, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		return filepath.Join(link, "config")
	case "azure hardlink":
		if err := os.Link(protected, path); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
	case "readonly":
		if err := os.WriteFile(path, []byte("unchanged\n"), 0o400); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	}
	return path
}
