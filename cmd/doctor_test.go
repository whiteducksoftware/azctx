package cmd_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/version"
)

type diagnosticRunner struct{ *testutil.FakeRunner }

func (diagnosticRunner) Path() string { return "/synthetic/az" }

func TestDoctorJSONGolden(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: `{"azure-cli":"2.60.0"}`}}}}
	deps := commandDependencies(t, fake)
	deps.Runner, deps.Platform, deps.ConfigDir = diagnosticRunner{fake}, platformMac, "/synthetic/azure"
	files := fstest.MapFS{
		profileFile: &fstest.MapFile{Data: []byte(`{"subscriptions":[
			{"user":{"type":"user","name":"private@example.invalid"}},
			{"user":{"type":"servicePrincipal","name":"private-client"}},
			{"user":{"type":"servicePrincipal","name":"systemAssignedIdentity"}},
			{"user":{"type":"servicePrincipal","name":"userAssignedIdentity"}},
			{"user":{"type":"secret-unknown-type","name":"secret-name"}}]}`)},
		azureConfigFile:   &fstest.MapFile{Data: []byte("[core]\npassword=secret-config\nenable_broker_on_mac=true\nlogin_experience_v2=false\n[cloud]\nname=AzureCloud")},
		retiredTenantFile: &fstest.MapFile{Data: []byte("secret-legacy-contents")},
	}
	deps.FS = diagnosticFS{files: files, t: t}
	original := version.Version
	version.Version = "2026.11.0"
	t.Cleanup(func() { version.Version = original })
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{doctorCommand, verboseFlag, outputFlag, jsonFormat}, deps, strings.NewReader(""), &stdout, &stderr)
	want, err := os.ReadFile("testdata/contract/doctor.golden")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || stderr.Len() != 0 || !bytes.Equal(stdout.Bytes(), want) {
		t.Fatalf("code=%d\nstdout=%s\nstderr=%s\nwant=%s", code, stdout.Bytes(), stderr.Bytes(), want)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].Attached || strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "private") {
		t.Fatalf("unsafe diagnostic: calls=%+v stdout=%q", fake.Calls, stdout.String())
	}
}

func TestDoctorIndependentDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, profile, config, versionOutput, want string
		versionError                               error
	}{
		{name: "no-profile", want: `"status":"missing"`},
		{name: "malformed-profile", profile: `{"subscriptions":secret-profile}`, want: `"status":"invalid"`},
		{name: "malformed-config", config: "[secret-config", want: `"status":"invalid"`},
		{name: "invalid-broker-bool", config: "[core]\nenable_broker_on_mac=secret-bool", want: `"broker":null`},
		{name: "invalid-login-bool", config: "[core]\nlogin_experience_v2=secret-bool", want: `"loginExperienceV2":null`},
		{name: "malformed-version", versionOutput: `{"azure-cli":"secret-version"}`, want: `"status":"unavailable"`},
		{name: "failed-version", versionError: errors.New("secret-process-error"), want: `"status":"unavailable"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := tc.versionOutput
			if output == "" {
				output = brokerVersionJSON
			}
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: output, Stderr: "secret-stderr"}, Err: tc.versionError}}}
			deps := commandDependencies(t, fake)
			deps.Platform = platformMac
			files := fstest.MapFS{}
			if tc.profile != "" {
				files[profileFile] = &fstest.MapFile{Data: []byte(tc.profile)}
			}
			if tc.config != "" {
				files[azureConfigFile] = &fstest.MapFile{Data: []byte(tc.config)}
			}
			deps.FS = diagnosticFS{files: files, t: t}
			var stdout, stderr bytes.Buffer
			code := cmd.Run([]string{doctorCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), tc.want) || strings.Contains(stdout.String(), "secret") || len(fake.Calls) != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
			}
		})
	}
}

// Doctor can stat the retired tenant file but may only open these two files.
type diagnosticFS struct {
	files fstest.MapFS
	t     *testing.T
}

func (f diagnosticFS) Open(name string) (fs.File, error) {
	if name != azureConfigFile && name != profileFile {
		f.t.Fatalf("doctor opened a forbidden Azure file: %q", name)
		return nil, fs.ErrPermission
	}
	return f.files.Open(name)
}

func (f diagnosticFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(f.files, name)
}
