package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAgentHookRealGuardReadOnly(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required for agent acceptance:", err)
	}
	hook, err := filepath.Abs(agentPlugin + "hooks/guard.py")
	if err != nil {
		t.Fatal(err)
	}
	_, azure, calls := fakeBinary(t, "[]")
	private := filepath.Dir(writeSelectionConfig(t, "protected: ["+tenantID+"]\n"))
	writeFixture(t, filepath.Join(private, "state.json"), "do-not-read-or-write\n")
	writeFixture(t, filepath.Join(private, "history.json"), "do-not-read-or-write\n")
	azureBefore, privateBefore := snapshotContextFiles(t, azure), snapshotContextFiles(t, private)
	root := testutil.WorkDir(t)
	cwd := filepath.Join(root, "nested")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, projectFile), projectYAML(productionID, tenantID))
	for _, command := range []string{"az group list; touch NEVER", "azd up", "terraform apply", "bicep build file.bicep"} {
		if code, messages := runAgentHook(t, python, hook, cwd, command); code != 0 || messages != "" {
			t.Fatalf("matching hook: %d %q", code, messages)
		}
	}
	assertAgentHookFailures(t, python, hook, cwd)
	assertAgentHookMissingAZCTX(t, python, hook, cwd, root)
	if !reflect.DeepEqual(azureBefore, snapshotContextFiles(t, azure)) || !reflect.DeepEqual(privateBefore, snapshotContextFiles(t, private)) {
		t.Fatal("hook mutated synthetic Azure/private files")
	}
	for _, path := range []string{calls, filepath.Join(cwd, "NEVER")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("hook executed Azure or proposed command: %s: %v", path, err)
		}
	}
}

func assertAgentHookFailures(t *testing.T, python, hook, cwd string) {
	t.Helper()
	// A nearer mismatch/malformed file must not use the matching ancestor.
	for _, tc := range []struct {
		project, status string
	}{
		{projectYAML(developmentID, tenantID), agentContextMismatch},
		{projectYAML(productionID, secondTenantID), agentContextMismatch},
		{"version: 1\nlogin: $(touch NEVER)\n", "exit 1"},
	} {
		writeFixture(t, filepath.Join(cwd, projectFile), tc.project)
		code, messages := runAgentHook(t, python, hook, cwd, "az group list")
		if code != 2 || !strings.Contains(messages, tc.status) || !strings.Contains(messages, "blocked") {
			t.Fatalf("failing hook: %d %q", code, messages)
		}
	}
}

func assertAgentHookMissingAZCTX(t *testing.T, python, hook, cwd, root string) {
	t.Helper()
	// Even without azctx, do not run the requested Bash command.
	t.Setenv("PATH", filepath.Join(root, "empty-path"))
	code, messages := runAgentHook(t, python, hook, cwd, "az group list; touch NEVER")
	if code != 0 || !strings.Contains(messages, "azctx not found") || len(strings.Split(strings.TrimSpace(messages), "\n")) != 1 {
		t.Fatalf("missing azctx hook: %d %q", code, messages)
	}
}

func runAgentHook(t *testing.T, python, hook, cwd, command string) (int, string) {
	t.Helper()
	input, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]string{"command": command},
		"cwd":             cwd,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, python, "-B", hook)
	var out, messages bytes.Buffer
	process.Stdin, process.Stdout, process.Stderr = bytes.NewReader(input), &out, &messages
	err = process.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || ctx.Err() != nil {
		t.Fatalf("hook stdout=%q stderr=%q timeout=%v", &out, &messages, ctx.Err())
	}
	return code, messages.String()
}
