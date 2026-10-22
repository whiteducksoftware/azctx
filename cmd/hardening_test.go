package cmd_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type tokenPathRunner struct{ *testutil.FakeRunner }

func (tokenPathRunner) Path() string {
	return "/synthetic/" + strings.Join([]string{"eyJhbGciOiJub25lIn0", "eyJzdWIiOiJmaXh0dXJlIn0", "signature"}, ".") + "/az"
}

func TestDoctorRedactsUntrustedDiagnosticValues(t *testing.T) {
	for _, format := range []string{"text", jsonFormat} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}}}
		deps := commandDependencies(t, fake)
		deps.Runner = tokenPathRunner{fake}
		deps.ConfigDir = "/synthetic/access_token=" + "synthetic-secret"
		deps.FS = fstest.MapFS{
			profileFile:       &fstest.MapFile{Data: []byte(`{}`)},
			azureConfigFile:   &fstest.MapFile{Data: []byte("[cloud]\nname=" + "Bearer" + " opaque-secret")},
			retiredTenantFile: &fstest.MapFile{Data: []byte("must not be opened")},
		}
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{doctorCommand, outputFlag, format}, deps, strings.NewReader(""), &stdout, &stderr)
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("doctor failed: status=%d stderr=%s", code, stderr.String())
		}
		for _, secret := range []string{"synthetic-secret", "opaque-secret", "eyJhbGci"} {
			if strings.Contains(stdout.String(), secret) {
				t.Fatalf("doctor leaked %q: %s", secret, stdout.String())
			}
		}
		if format == jsonFormat && !json.Valid(stdout.Bytes()) {
			t.Fatalf("redaction broke JSON: %s", stdout.String())
		}
	}
}

func TestLoginExplicitPassthroughRemainsUnvalidated(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}}, {},
	}}
	deps := commandDependencies(t, fake)
	// A domain is valid Azure CLI input but deliberately not an azctx GUID.
	args := []string{loginCommand, argumentDelimiter, tenantFlag, metadataTenantDomain, "--scope", "https://management.azure.com/.default"}
	var stdout, stderr bytes.Buffer
	code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || len(fake.Calls) != 2 || strings.Join(fake.Calls[1].Args, "\n") != strings.Join(append([]string{loginCommand}, args[2:]...), "\n") {
		t.Fatalf("passthrough was changed: status=%d calls=%+v stderr=%s", code, fake.Calls, stderr.String())
	}
}

func TestExecRefusesImplicitCurrentDirectoryExecutable(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName(childFixtureName)), childFixturePackage)
	deps := commandDependencies(t, &testutil.FakeRunner{})
	t.Chdir(dir)
	t.Setenv("PATH", ".")
	t.Setenv("GODEBUG", "execerrdot=1")
	var stdout, stderr bytes.Buffer
	status := cmd.Run([]string{execSubcommand, productionID, argumentDelimiter, childFixtureName}, deps, strings.NewReader(""), &stdout, &stderr)
	if status != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "current directory") {
		t.Fatalf("exec bypassed current-directory refusal: status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
}

func TestUntrustedProfileTenantNeverReachesLogin(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		for _, id := range []string{"bad-id", "&echo injected", "%COMSPEC%"} {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Platform = platform
			data, err := json.Marshal(map[string]any{"subscriptions": []map[string]any{{
				idFormat: productionID, "tenantId": id, "tenantDisplayName": "Directory", "environmentName": azureCloudName,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
			var stdout, stderr bytes.Buffer
			status := cmd.Run([]string{loginCommand, tenantFlag, "Directory"}, deps, strings.NewReader(""), &stdout, &stderr)
			if status != 1 || len(fake.Calls) != 0 {
				t.Fatalf("platform=%s invalid profile tenant %q reached az: status=%d calls=%+v", platform, id, status, fake.Calls)
			}
		}
	}
}

func TestUntrustedProfileIDsNeverReachAzureCLI(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		for _, id := range []string{"bad-id", "&echo injected", "%COMSPEC%", `x" & whoami & "`} {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Platform = platform
			data, err := json.Marshal(map[string]any{"subscriptions": []map[string]any{{
				idFormat: id, "tenantId": tenantID, nameFormat: productionName, "environmentName": azureCloudName,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: data}}
			var stdout, stderr bytes.Buffer
			status := cmd.Run([]string{argumentDelimiter, productionName}, deps, strings.NewReader(""), &stdout, &stderr)
			if status != 1 || len(fake.Calls) != 0 {
				t.Fatalf("platform=%s invalid profile ID %q reached az: status=%d calls=%+v", platform, id, status, fake.Calls)
			}
		}
	}
}
