package cmd_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const brokerMac = "enable_broker_on_mac"

const brokerWindows = "enable_broker_on_windows"

func TestBrokerConfirmedSequence(t *testing.T) {
	for _, platform := range []string{platformWindows, platformMac} {
		for _, action := range []string{brokerEnable, brokerDisable} {
			t.Run(platform+"/"+action, func(t *testing.T) {
				fake := &testutil.FakeRunner{Responses: []testutil.Response{
					{Result: azcli.Result{Stdout: brokerVersionJSON}},
					{},
					{},
					{Result: azcli.Result{Stdout: "Azure login data\n", Stderr: "Azure login prompt\n"}},
				}}
				deps := commandDependencies(t, fake)
				deps.Platform = platform
				key := brokerWindows
				if platform == platformMac {
					key = brokerMac
				}

				// Start in the opposite mode, with no valid profile required.
				before, after := "true", boolFalse
				if action == brokerEnable {
					before, after = after, before
				}
				// A matching environment override must not hide a persistence request.
				t.Setenv("AZURE_CORE_"+strings.ToUpper(key), after)
				deps.FS = fstest.MapFS{azureConfigFile: &fstest.MapFile{Data: []byte("[core]\n" + key + "=" + before)}}
				var stdout, stderr bytes.Buffer
				code := cmd.Run([]string{authCommand, brokerCommand, action, confirmFlag, noInputFlag}, deps, strings.NewReader(""), &stdout, &stderr)
				want := []testutil.Call{
					{Args: []string{versionCommand, outputFlag, jsonFormat}},
					{Args: []string{accountCommand, "clear"}},
					{Args: []string{azureConfigFile, setCommand, "core." + key + "=" + after}},
					{Args: []string{loginCommand}, Attached: true, Env: []string{childPickerOff}},
				}
				if code != 0 || !reflect.DeepEqual(fake.Calls, want) || stdout.String() != "Azure login data\n" ||
					!strings.Contains(stderr.String(), "Azure login prompt") {
					t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
				}
				if platform == platformMac && (!strings.Contains(stderr.String(), "preview") || !strings.Contains(stderr.String(), "enrolled")) {
					t.Fatalf("missing preview warning: %q", stderr.String())
				}
			})
		}
	}
}

func TestBrokerSafetyGates(t *testing.T) {
	for _, tc := range []struct {
		name, platform, version, input, override string
		args                                     []string
		tty                                      bool
		wantCode, wantCalls                      int
	}{
		{name: "unsupported", platform: platformLinux, version: brokerVersion, args: []string{brokerEnable, confirmFlag}, wantCode: 1},
		{name: "old-windows", platform: platformWindows, version: oldAzureVersion, args: []string{brokerEnable, confirmFlag}, wantCode: 1, wantCalls: 1},
		{name: "old-mac-enable", platform: platformMac, version: oldMacVersion, args: []string{brokerEnable, confirmFlag}, wantCode: 1, wantCalls: 1},
		{name: "old-mac-disable", platform: platformMac, version: oldMacVersion, args: []string{brokerDisable, confirmFlag}, wantCode: 1, wantCalls: 1},
		{name: "redirected", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable}, input: "yes\n", wantCode: 1, wantCalls: 1},
		{name: "no-input", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable, noInputFlag}, tty: true, input: "yes\n", wantCode: 1, wantCalls: 1},
		{name: "declined", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable}, tty: true, input: "n\n", wantCalls: 1},
		{name: "blank", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable}, tty: true, input: "\n", wantCalls: 1},
		{name: "eof", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable}, tty: true, wantCode: 1, wantCalls: 1},
		{name: "override-conflict", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable, confirmFlag}, override: boolFalse, wantCode: 1, wantCalls: 1},
		{name: "invalid-setting", platform: platformWindows, version: brokerVersion, args: []string{brokerEnable, confirmFlag}, override: "secret-not-a-bool", wantCode: 1},
		{name: "bad-format", platform: platformWindows, args: []string{brokerEnable, confirmFlag, outputFlag, jsonFormat}, wantCode: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: `{"azure-cli":"` + tc.version + `"}`}}}}
			deps := commandDependencies(t, fake)
			deps.Platform, deps.FS = tc.platform, fstest.MapFS{}
			deps.StdinTTY = func() bool { return tc.tty }
			if tc.override != "" {
				t.Setenv("AZURE_CORE_ENABLE_BROKER_ON_WINDOWS", tc.override)
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(append([]string{authCommand, brokerCommand}, tc.args...), deps, strings.NewReader(tc.input), &stdout, &stderr)
			if code != tc.wantCode || len(fake.Calls) != tc.wantCalls || stdout.Len() != 0 || strings.Contains(stderr.String(), "secret-not-a-bool") {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}

func TestBrokerStopsAtEachFailedStep(t *testing.T) {
	for _, step := range []int{1, 2, 3} {
		for _, failure := range []error{errors.New("process failed"), ui.ErrInterrupted} {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {}, {}, {}}}
			fake.Responses[step].Err = failure
			deps := commandDependencies(t, fake)
			deps.Platform, deps.FS = platformWindows, fstest.MapFS{}
			var stdout, stderr bytes.Buffer
			code := cmd.Run([]string{authCommand, brokerCommand, brokerDisable, confirmFlag}, deps, strings.NewReader(""), &stdout, &stderr)
			wantCode := 1
			if errors.Is(failure, ui.ErrInterrupted) {
				wantCode = 130
			}
			if code != wantCode || len(fake.Calls) != step+1 || stdout.Len() != 0 {
				t.Fatalf("step=%d code=%d calls=%+v stdout=%q stderr=%q", step, code, fake.Calls, stdout.String(), stderr.String())
			}
		}
	}
}
