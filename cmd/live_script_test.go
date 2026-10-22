package cmd_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

// TestLiveAzureScriptOffline runs the manual live script on every CI platform
// against the portable fake executable standing in for azctx. It proves the
// refusal gates, command sequence and restoration, never live Azure acceptance.
func TestLiveAzureScriptOffline(t *testing.T) {
	for _, variant := range []string{"success", "no-opt-in", "foreign-tenant", "bad-schema", "guard-failure", "restore-failure"} {
		t.Run(variant, func(t *testing.T) {
			steps := liveScriptFixtures(t)
			optIn, wantCode, wantCalls, wantText := true, 0, len(steps), "PASS host Azure CLI"
			switch variant {
			case "no-opt-in":
				optIn, wantCode, wantCalls, wantText = false, 2, 0, "REFUSED"
			case "foreign-tenant":
				current, ok := steps[0][fixtureStdout].(string)
				if !ok {
					t.Fatal("fixture current output must be text")
				}
				steps[0][fixtureStdout] = strings.ReplaceAll(current, tenantID, secondTenantID)
				wantCode, wantCalls, wantText = 2, 1, "nothing was changed"
			case "bad-schema":
				steps[1][fixtureStdout] = `{"schemaVersion":2,"subscriptions":[]}`
				wantCode, wantCalls, wantText = 1, 2, "unchanged"
			case "guard-failure":
				steps[6]["exitCode"] = 0
				steps = append(steps[:7], steps[len(steps)-2:]...)
				wantCode, wantCalls, wantText = 1, len(steps), "initial context restored"
			case "restore-failure":
				steps[len(steps)-2]["exitCode"] = 1
				wantCode, wantCalls, wantText = 3, len(steps)-1, "RESTORE FAILED"
			}
			code, output, record, work := runLiveScript(t, steps, optIn)
			if code != wantCode || !strings.Contains(output, wantText) {
				t.Fatalf("live script: code=%d want %d output=%q", code, wantCode, output)
			}
			if strings.Contains(output, "PASS") != (wantCode == 0) {
				t.Fatalf("dishonest acceptance: %q", output)
			}
			if wantCalls == 0 {
				if _, err := os.Stat(record); !os.IsNotExist(err) {
					t.Fatalf("refusal started azctx: %v", err)
				}
			} else {
				assertFixtureCallCount(t, record, wantCalls)
				assertLiveScriptEnvironment(t, record, work)
			}
			if entries, err := os.ReadDir(work); err != nil || len(entries) != 0 {
				t.Fatalf("private work directory left behind: %v, %v", entries, err)
			}
		})
	}
}

func runLiveScript(t *testing.T, steps []map[string]any, optIn bool) (int, string, string, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	binary, _, record := fakeBinary(t, string(data))
	t.Setenv("FAKEAZ_ENV_KEYS", "AZCTX_CONFIG_DIR,AZCTX_NO_INPUT,AZCTX_NO_UPDATE_CHECK")
	work := t.TempDir()
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(key, work)
	}
	for _, key := range []string{"GITHUB_ACTIONS", "TF_BUILD"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{
		"-B", filepath.Join("..", "scripts", "live-azure-test.py"),
		"--binary", filepath.Join(filepath.Dir(binary), testutil.ExecutableName("az")),
		tenantFlag, tenantID, subscriptionFlag, productionID, subscriptionFlag, developmentID,
	}
	if optIn {
		args = append(args, "--opt-in-live-azure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, python, args...)
	output, err := process.CombinedOutput()
	if process.ProcessState == nil || ctx.Err() != nil {
		t.Fatalf("run live script: %v, %v", err, ctx.Err())
	}
	return process.ProcessState.ExitCode(), string(output), record, work
}

func assertLiveScriptEnvironment(t *testing.T, record, work string) {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var call struct{ Env []string }
		if err := json.Unmarshal([]byte(line), &call); err != nil || len(call.Env) != 3 {
			t.Fatalf("recorded call %q: %v", line, err)
		}
		config := strings.TrimPrefix(call.Env[0], "AZCTX_CONFIG_DIR=")
		relative, err := filepath.Rel(work, config)
		if err != nil || relative == "." || strings.HasPrefix(relative, "..") ||
			call.Env[1] != "AZCTX_NO_INPUT=1" || call.Env[2] != "AZCTX_NO_UPDATE_CHECK=1" {
			t.Fatalf("live script must isolate azctx state and disable input/updates: %q", call.Env)
		}
	}
}

func liveScriptFixtures(t *testing.T) []map[string]any {
	t.Helper()
	list, err := os.ReadFile(filepath.Join("testdata", "contract", "list.golden"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join("testdata", "contract", "current.golden"))
	if err != nil {
		t.Fatal(err)
	}
	steps := []map[string]any{
		{fixtureArgs: []string{currentFlag, outputFlag, jsonFormat}, fixtureStdout: string(current)},
		{fixtureArgs: []string{listCommand, outputFlag, jsonFormat}, fixtureStdout: string(list)},
	}
	for _, id := range []string{productionID, developmentID} {
		other := developmentID
		if id == developmentID {
			other = productionID
		}
		exports := "export ARM_SUBSCRIPTION_ID='" + id + "'\n" +
			"export ARM_TENANT_ID='" + tenantID + "'\n" +
			"export AZURE_SUBSCRIPTION_ID='" + id + "'\n" +
			"export AZURE_TENANT_ID='" + tenantID + "'\n"
		active := strings.ReplaceAll(string(current), productionID, id)
		steps = append(steps,
			map[string]any{fixtureArgs: []string{outputFlag, jsonFormat, argumentDelimiter, id}},
			map[string]any{fixtureArgs: []string{currentFlag, outputFlag, jsonFormat}, fixtureStdout: active},
			map[string]any{fixtureArgs: []string{envCommand, shellFlag, envBashSyntax}, fixtureStdout: exports},
			map[string]any{fixtureArgs: []string{guardCommand, subscriptionFlag, id, tenantFlag, tenantID}},
			map[string]any{
				fixtureArgs:   []string{guardCommand, subscriptionFlag, other, tenantFlag, tenantID, outputFlag, jsonFormat},
				fixtureStderr: "{\"code\":\"context_mismatch\",\"exit_code\":4}\n", "exitCode": 4,
			},
			map[string]any{fixtureArgs: []string{currentFlag, outputFlag, jsonFormat}, fixtureStdout: active},
		)
	}
	return append(steps,
		map[string]any{fixtureArgs: []string{outputFlag, jsonFormat, argumentDelimiter, productionID}},
		map[string]any{fixtureArgs: []string{currentFlag, outputFlag, jsonFormat}, fixtureStdout: string(current)},
	)
}
