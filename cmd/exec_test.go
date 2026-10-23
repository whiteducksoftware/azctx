package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	execSubcommand      = "exec"
	childFixtureName    = "contextchild"
	childFixturePackage = "github.com/whiteducksoftware/azctx/internal/testutil/contextchild"
	childStderr         = "child stderr\n"
	childModeEnv        = "AZCTX_TEST_CHILD_MODE"
	childReadyEnv       = "AZCTX_TEST_CHILD_READY"
	childWaitMode       = "wait"
	childTrapMode       = "trap"
	childExitMode       = "exit"
	childJSONExitMode   = "json-exit"
)

func TestExecAttachedArgvEnvironmentAndExit(t *testing.T) {
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, testutil.ExecutableName(childFixtureName))
	testutil.Build(t, helper, childFixturePackage)
	for _, mode := range []string{"echo", childExitMode, childJSONExitMode} {
		t.Run(mode, func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			configPath := writeSelectionConfig(t, "aliases:\n  dev: "+developmentID+"\nprotected: ["+developmentID+"]\n")
			t.Setenv("PATH", dir)
			childMode := mode
			if mode == childJSONExitMode {
				childMode = childExitMode
			}
			t.Setenv(childModeEnv, childMode)
			t.Setenv("AZCTX_TEST_INHERITED", "preserved")
			for _, key := range []string{"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID"} {
				t.Setenv(key, "parent-value")
			}
			argv := []string{"space value", `$(touch nope); & | " '`, "a\nb", outputFlag + "=" + jsonFormat, noInputFlag, ""}
			args := append([]string{execSubcommand, selectionAlias, argumentDelimiter, childFixtureName}, argv...)
			if mode == childJSONExitMode {
				args = append([]string{outputFlag, jsonFormat}, args...)
			}
			var out, messages bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader("attached input\n"), &out, &messages)
			assertExecReadOnly(t, configPath, fake)
			if childMode == childExitMode {
				if code != 37 || out.Len() != 0 || messages.String() != childStderr {
					t.Fatalf("exit: code=%d stdout=%q stderr=%q", code, &out, &messages)
				}
				return
			}
			assertChildEcho(t, code, &out, &messages, argv)
		})
	}
}

func assertExecReadOnly(t *testing.T, configPath string, fake *testutil.FakeRunner) {
	t.Helper()
	if os.Getenv("ARM_SUBSCRIPTION_ID") != "parent-value" {
		t.Fatal("child environment contaminated parent")
	}
	entries, err := os.ReadDir(filepath.Dir(configPath))
	if err != nil || len(entries) != 1 || entries[0].Name() != envConfigFilename || len(fake.Calls) != 0 {
		t.Fatalf("exec mutated state or invoked Azure: %v, %v, %+v", entries, err, fake.Calls)
	}
}

func assertChildEcho(t *testing.T, code int, out, messages *bytes.Buffer, argv []string) {
	t.Helper()
	var got struct {
		Args  []string          `json:"args"`
		Env   map[string]string `json:"env"`
		Stdin string            `json:"stdin"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("code=%d stdout=%q stderr=%q: %v", code, out, messages, err)
	}
	wantEnv := map[string]string{
		"ARM_SUBSCRIPTION_ID": developmentID, "ARM_TENANT_ID": tenantID,
		"AZURE_SUBSCRIPTION_ID": developmentID, "AZURE_TENANT_ID": tenantID,
		"AZCTX_TEST_INHERITED": "preserved",
	}
	if code != 0 || !reflect.DeepEqual(got.Args, argv) || !reflect.DeepEqual(got.Env, wantEnv) ||
		got.Stdin != "attached input\n" || messages.String() != childStderr {
		t.Fatalf("code=%d child=%+v stderr=%q", code, got, messages)
	}
}
