package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

// The injected Azure process, not azctx, owns initial profile creation.
type firstLoginRunner struct {
	fake      *testutil.FakeRunner
	configDir string
	exists    bool
}

const supportedAzureVersion = "2.61.0"

func (r firstLoginRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	if _, err := os.Stat(filepath.Join(r.configDir, profileFile)); !os.IsNotExist(err) {
		return azcli.Result{}, errors.Join(errors.New("profile must be absent before Azure CLI"), err)
	}
	if !r.exists {
		if _, err := os.Stat(r.configDir); !os.IsNotExist(err) {
			return azcli.Result{}, errors.Join(errors.New("directory must be absent before Azure CLI"), err)
		}
	}
	result, err := r.fake.Run(ctx, args, options)
	if err == nil && len(args) > 0 && args[0] == loginCommand {
		if err := os.MkdirAll(r.configDir, 0o700); err != nil {
			return result, err
		}
		err = os.WriteFile(filepath.Join(r.configDir, profileFile), []byte(fixtureProfile), 0o600)
	}
	return result, err
}

func TestAzureCLIFirstLogin(t *testing.T) {
	for _, existingDir := range []bool{false, true} {
		for _, version := range []string{"2.60.0", supportedAzureVersion} {
			for _, force := range []bool{false, true} {
				t.Run(strings.Join([]string{map[bool]string{false: "missing-dir", true: "missing-profile"}[existingDir], version, map[bool]string{false: "normal", true: "force-mfa"}[force]}, "/"), func(t *testing.T) {
					testFirstLogin(t, existingDir, version, force)
				})
			}
		}
	}
}

func testFirstLogin(t *testing.T, existingDir bool, version string, force bool) {
	t.Helper()
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: `{"azure-cli":"` + version + `"}`}}, {},
	}}
	deps := commandDependencies(t, fake)
	deps.FS = nil
	deps.ConfigDir = filepath.Join(testutil.WorkDir(t), "azure")
	if existingDir {
		if err := os.Mkdir(deps.ConfigDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	deps.Runner = firstLoginRunner{fake: fake, configDir: deps.ConfigDir, exists: existingDir}
	args := []string{loginCommand}
	if force {
		args = append(args, "--force-mfa")
	}
	args = append(args, argumentDelimiter, deviceCodeFlag)
	var stdout, stderr bytes.Buffer
	code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr)
	supported := version == supportedAzureVersion
	if supported {
		if code != 0 || !strings.Contains(stderr.String(), "Successfully fetched 1 tenants and 2 subscriptions.") {
			t.Fatalf("first login: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	} else if code != 1 || !strings.Contains(stderr.String(), "2.61.0 or newer is required") {
		t.Fatalf("version gate: code=%d stderr=%q", code, stderr.String())
	}
	assertFirstLoginCalls(t, fake, supported)
	assertFirstLoginFiles(t, deps.ConfigDir, existingDir, supported)
}

func assertFirstLoginCalls(t *testing.T, fake *testutil.FakeRunner, supported bool) {
	t.Helper()
	wantCalls := [][]string{{versionCommand, outputFlag, jsonFormat}}
	if supported {
		wantCalls = append(wantCalls, []string{loginCommand, deviceCodeFlag})
	}
	var gotCalls [][]string
	for _, call := range fake.Calls {
		gotCalls = append(gotCalls, call.Args)
	}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("az calls=%q want=%q", gotCalls, wantCalls)
	}
	if fake.Calls[0].Attached || len(fake.Calls[0].Env) != 0 {
		t.Fatalf("version inherited login options: %+v", fake.Calls[0])
	}
	if supported && (!fake.Calls[1].Attached || !reflect.DeepEqual(fake.Calls[1].Env, []string{"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"})) {
		t.Fatalf("login lost terminal/environment: %+v", fake.Calls[1])
	}
}

func assertFirstLoginFiles(t *testing.T, configDir string, existingDir, supported bool) {
	t.Helper()
	entries, err := os.ReadDir(configDir)
	switch {
	case supported:
		if err != nil || len(entries) != 1 || entries[0].Name() != profileFile {
			t.Fatalf("unexpected Azure files: %+v, %v", entries, err)
		}
	case existingDir:
		if err != nil || len(entries) != 0 {
			t.Fatalf("azctx created Azure files: %+v, %v", entries, err)
		}
	default:
		if !os.IsNotExist(err) {
			t.Fatalf("azctx created Azure directory: %+v, %v", entries, err)
		}
	}
}

func TestAzureCLIMissingProfileStillFailsReads(t *testing.T) {
	for _, existingDir := range []bool{false, true} {
		for _, args := range [][]string{{listCommand}, {currentFlag}, {productionID}} {
			t.Run(strings.Join(args, " ")+map[bool]string{true: "/missing-profile", false: "/missing-dir"}[existingDir], func(t *testing.T) {
				fake := &testutil.FakeRunner{}
				deps := commandDependencies(t, fake)
				deps.FS = nil
				deps.ConfigDir = filepath.Join(testutil.WorkDir(t), "azure")
				if existingDir {
					if err := os.Mkdir(deps.ConfigDir, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				var stdout, stderr bytes.Buffer
				if code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 1 ||
					stdout.Len() != 0 || !strings.Contains(stderr.String(), "az login") || len(fake.Calls) != 0 {
					t.Fatalf("missing profile: code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
				}
			})
		}
	}
}

func TestAzureCLILoginPreservesMalformedSettingsDiagnostics(t *testing.T) {
	for _, name := range []string{azureConfigFile, cloudsConfigFile} {
		t.Run(name, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.FS = fstest.MapFS{name: &fstest.MapFile{Data: []byte("[broken")}}
			var stdout, stderr bytes.Buffer
			if code := cmd.Run([]string{loginCommand}, deps, bytes.NewReader(nil), &stdout, &stderr); code != 1 ||
				!strings.Contains(stderr.String(), "malformed "+name+" at line 1") || len(fake.Calls) != 0 {
				t.Fatalf("malformed settings: code=%d stderr=%q calls=%+v", code, stderr.String(), fake.Calls)
			}
		})
	}
}

func TestAzureCLILoginRequiresAzureOwnedProfile(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: `{"azure-cli":"` + supportedAzureVersion + `"}`}}, {},
	}}
	deps := commandDependencies(t, fake)
	files := fstest.MapFS{}
	deps.FS = azureReadFS{t: t, files: files}
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand}, deps, bytes.NewReader(nil), &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), profileFile+" is not a valid file") || len(fake.Calls) != 2 || len(files) != 0 {
		t.Fatalf("post-login missing profile: code=%d stderr=%q calls=%+v files=%+v", code, stderr.String(), fake.Calls, files)
	}
}
