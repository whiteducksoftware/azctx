package cmd_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestLoginPlatformAndHeadlessMessages(t *testing.T) {
	for _, tc := range []struct {
		platform, config, env, version, want string
		device                               bool
	}{
		{platform: platformWindows, version: supportedAzureVersion, want: "WAM"},
		{platform: platformWindows, version: brokerVersion, config: "enable_broker_on_windows=off", want: browserPrompt},
		{platform: platformMac, version: brokerVersion, config: "enable_broker_on_mac=yes", want: "macOS broker account picker"},
		{platform: platformMac, version: oldMacVersion, config: "enable_broker_on_mac=yes", want: browserPrompt},
		{platform: platformMac, version: brokerVersion, want: browserPrompt},
		{platform: platformLinux, version: brokerVersion, want: headlessMessage},
		{platform: platformWindows, version: brokerVersion, env: sshEnvironment, want: headlessMessage},
		{platform: platformMac, version: brokerVersion, env: "CODESPACES", want: headlessMessage},
		{platform: platformWindows, version: brokerVersion, env: "REMOTE_CONTAINERS", want: headlessMessage},
		{platform: platformWindows, version: brokerVersion, env: sshEnvironment, device: true, want: "device code"},
	} {
		t.Run(tc.platform+"/"+tc.config+"/"+tc.env, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: `{"azure-cli":"` + tc.version + `"}`}}, {}}}
			deps := commandDependencies(t, fake)
			deps.Platform = tc.platform
			for _, key := range []string{"DISPLAY", "WAYLAND_DISPLAY", sshEnvironment, "CODESPACES", "REMOTE_CONTAINERS", "AZURE_CORE_ENABLE_BROKER_ON_WINDOWS", "AZURE_CORE_ENABLE_BROKER_ON_MAC"} {
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			if tc.env != "" {
				t.Setenv(tc.env, "") // Presence alone is sufficient.
			}
			files, ok := deps.FS.(fstest.MapFS)
			if !ok {
				t.Fatal("fixture is not a MapFS")
			}
			files[azureConfigFile] = &fstest.MapFile{Data: []byte("[core]\n" + tc.config)}
			args := []string{loginCommand}
			if tc.device {
				args = append(args, deviceCodeFlag)
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || !strings.Contains(stderr.String(), tc.want) || (tc.device && strings.Contains(stderr.String(), headlessMessage)) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if tc.want == headlessMessage && strings.Index(stderr.String(), deviceCodeFlag) > strings.Index(stderr.String(), "Logging in") {
				t.Fatalf("late headless suggestion: %q", stderr.String())
			}
		})
	}
}

func TestFriendlyLoginFlags(t *testing.T) {
	for _, tenant := range []string{tenantID, "exinv", "EXAMPLE.INVALID"} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{
			{Result: azcli.Result{Stdout: brokerVersionJSON}}, {},
		}}
		deps := commandDependencies(t, fake)
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{loginCommand, tenantFlag, tenant, deviceCodeFlag}, deps, strings.NewReader(""), &stdout, &stderr)
		if code != 0 || len(fake.Calls) != 2 {
			t.Fatalf("tenant=%q code=%d stderr=%q calls=%+v", tenant, code, stderr.String(), fake.Calls)
		}
		want := []string{loginCommand, deviceCodeFlag, tenantFlag, tenantID}
		if !reflect.DeepEqual(fake.Calls[1].Args, want) || !fake.Calls[1].Attached ||
			!reflect.DeepEqual(fake.Calls[1].Env, []string{childPickerOff}) ||
			!strings.Contains(stderr.String(), "device code") || strings.Contains(stderr.String(), "check your browser") {
			t.Fatalf("login argv/messages: calls=%+v stderr=%q", fake.Calls, stderr.String())
		}
	}
}

func TestLoginTenantFailuresNeverStartAzure(t *testing.T) {
	for _, query := range []string{"", "not-found", "--evil", "example"} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(strings.TrimSuffix(fixtureProfile, "]}") +
			`,{"id":"33333333-3333-3333-3333-333333333333","tenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","tenantDisplayName":"Other","tenantDefaultDomain":"example.other","environmentName":"AzureCloud"}]}`)}}
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{loginCommand, "--tenant=" + query}, deps, strings.NewReader(""), &stdout, &stderr)
		if code == 0 || len(fake.Calls) != 0 || stdout.Len() != 0 {
			t.Fatalf("query=%q code=%d stdout=%q stderr=%q calls=%+v", query, code, stdout.String(), stderr.String(), fake.Calls)
		}
	}
}

func TestLoginResolvesMetadataOnAnyTenantRecord(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}}, {},
	}}
	deps := commandDependencies(t, fake)
	deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(`{"subscriptions":[
				{"tenantId":"` + tenantID + `","environmentName":"AzureCloud"},
				{"tenantId":"` + strings.ToUpper(tenantID) + `","environmentName":"AzureCloud","tenantDisplayName":"Unique Company"}]}`)}}
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand, tenantFlag, "uqcmp"}, deps, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || len(fake.Calls) != 2 || !strings.EqualFold(fake.Calls[1].Args[2], tenantID) {
		t.Fatalf("code=%d calls=%+v stderr=%q", code, fake.Calls, stderr.String())
	}
}
