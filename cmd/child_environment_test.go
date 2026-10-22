package cmd_test

import (
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	childConfigDirPrefix = "AZURE_CONFIG_DIR="
	childPickerOn        = "on"
)

func TestProfileIndependentChildConfigDir(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		calls int
	}{
		{"enable", []string{authCommand, brokerCommand, brokerEnable, confirmFlag}, 4},
		{"disable", []string{authCommand, brokerCommand, brokerDisable, confirmFlag}, 4},
		{binaryBrokerStatus, []string{authCommand, brokerCommand, binaryBrokerStatus}, 1},
		{"doctor", []string{doctorCommand, outputFlag, jsonFormat}, 1},
	} {
		for _, configDir := range []string{"", "explicit azure"} {
			t.Run(tc.name+"/"+configDir, func(t *testing.T) {
				fake := &testutil.FakeRunner{Responses: []testutil.Response{
					{Result: azcli.Result{Stdout: brokerVersionJSON}}, {}, {}, {},
				}}
				deps := commandDependencies(t, fake)
				deps.ConfigDir, deps.Platform, deps.FS = configDir, platformWindows, fstest.MapFS{}
				t.Setenv("AZURE_CONFIG_DIR", "inherited azure")
				t.Setenv("AZURE_CORE_LOGIN_EXPERIENCE_V2", childPickerOn)
				t.Setenv("AZURE_EXTENSION_USE_DYNAMIC_INSTALL", "yes_without_prompt")
				code := cmd.Run(tc.args, deps, strings.NewReader(""), io.Discard, io.Discard)
				if code != 0 || len(fake.Calls) != tc.calls {
					t.Fatalf("code=%d calls=%+v", code, fake.Calls)
				}
				for _, call := range fake.Calls {
					var want []string
					if call.Args[0] == loginCommand {
						want = append(want, childPickerOff)
						if !call.Attached {
							t.Error("broker login lost attached terminal")
						}
					}
					if configDir != "" {
						want = append(want, childConfigDirPrefix+configDir)
					}
					if !slices.Equal(call.Env, want) {
						t.Errorf("az %q child env=%q, want %q", call.Args, call.Env, want)
					}
				}
				assertInheritedAzureEnvironment(t)
			})
		}
	}
}

func assertInheritedAzureEnvironment(t *testing.T) {
	t.Helper()
	for key, want := range map[string]string{
		"AZURE_CONFIG_DIR":                    "inherited azure",
		"AZURE_CORE_LOGIN_EXPERIENCE_V2":      childPickerOn,
		"AZURE_EXTENSION_USE_DYNAMIC_INSTALL": "yes_without_prompt",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("parent %s=%q, want %q", key, got, want)
		}
	}
}
