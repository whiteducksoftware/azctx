package cmd_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestBrokerStatusMatrix(t *testing.T) {
	for _, tc := range []struct {
		platform, version, setting, want string
	}{
		{platform: platformWindows, version: supportedAzureVersion, want: "enabled=true, available=true"},
		{platform: platformWindows, version: oldAzureVersion, want: "enabled=true, available=false"},
		{platform: platformWindows, version: brokerVersion, setting: brokerWindows + "=off", want: "enabled=false, available=true"},
		{platform: platformMac, version: brokerVersion, want: "enabled=false, available=true"},
		{platform: platformMac, version: brokerVersion, setting: brokerMac + "=yes", want: "enabled=true, available=true"},
		{platform: platformMac, version: oldMacVersion, setting: brokerMac + "=yes", want: "enabled=true, available=false"},
		{platform: platformLinux, version: brokerVersion, want: "unsupported"},
	} {
		t.Run(tc.platform+"/"+tc.version+"/"+tc.setting, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: `{"azure-cli":"` + tc.version + `"}`}}}}
			deps := commandDependencies(t, fake)
			deps.Platform = tc.platform
			deps.FS = fstest.MapFS{azureConfigFile: &fstest.MapFile{Data: []byte("[core]\n" + tc.setting)}}
			var stdout, stderr bytes.Buffer
			code := cmd.Run([]string{authCommand, brokerCommand, "status"}, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 || len(fake.Calls) != 1 || !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
			}
			if tc.platform == platformMac && (!strings.Contains(stdout.String(), "preview") || !strings.Contains(stdout.String(), "enrolled")) {
				t.Fatalf("preview prerequisites absent: %q", stdout.String())
			}
		})
	}
}

func TestAuthDiagnosticCancellation(t *testing.T) {
	for _, args := range [][]string{{doctorCommand, outputFlag, jsonFormat}, {authCommand, brokerCommand, "status"}, {authCommand, brokerCommand, brokerEnable, confirmFlag}} {
		for _, failure := range []error{ui.ErrInterrupted, context.Canceled} {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stderr: "secret-version-error"}, Err: failure}}}
			deps := commandDependencies(t, fake)
			deps.Platform, deps.FS = platformWindows, fstest.MapFS{}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 130 || stdout.Len() != 0 {
				t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
			}
			if args[0] == doctorCommand && strings.Contains(stderr.String(), "secret") {
				t.Fatalf("doctor echoed a raw interrupted version diagnostic: %q", stderr.String())
			}
		}
	}
}

func TestBrokerEnvironmentNoInputGates(t *testing.T) {
	for _, key := range []string{"CI", "AZCTX_NO_INPUT"} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}}}
		deps := commandDependencies(t, fake)
		deps.Platform, deps.FS = platformWindows, fstest.MapFS{}
		t.Setenv(key, "1")
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{authCommand, brokerCommand, brokerEnable}, deps, strings.NewReader("yes\n"), &stdout, &stderr)
		if code != 1 || len(fake.Calls) != 1 || strings.Contains(stderr.String(), "Continue?") {
			t.Fatalf("gate=%s code=%d calls=%+v stderr=%q", key, code, fake.Calls, stderr.String())
		}
	}
}

func TestDoctorMissingAzureAndDirectory(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	deps.Runner, deps.FS = nil, nil
	deps.ConfigDir = testutil.WorkDir(t) + "/missing-azure"
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{doctorCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"status":"unavailable"`) ||
		!strings.Contains(stdout.String(), `"status":"missing"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(deps.ConfigDir); !os.IsNotExist(err) {
		t.Fatalf("doctor created Azure directory: %v", err)
	}
}
