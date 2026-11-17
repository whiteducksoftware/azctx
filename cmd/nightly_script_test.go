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

func TestNightlySmokeScriptOffline(t *testing.T) {
	for _, variant := range []string{"success", "bad-schema", "foreign-tenant", "guard-failure"} {
		t.Run(variant, func(t *testing.T) {
			steps := nightlyScriptFixtures(t)
			wantCode, wantCalls := 0, len(steps)
			switch variant {
			case "bad-schema":
				steps[0][fixtureStdout] = `{"schemaVersion":2,"subscriptions":[]}`
				wantCode, wantCalls = 1, 1
			case "foreign-tenant":
				list, ok := steps[0][fixtureStdout].(string)
				if !ok {
					t.Fatal("fixture list output must be text")
				}
				steps[0][fixtureStdout] = strings.ReplaceAll(list, tenantID, secondTenantID)
				wantCode, wantCalls = 1, 1
			case "guard-failure":
				steps[6]["exitCode"] = 0
				steps = append(steps[:7], steps[len(steps)-2:]...)
				wantCode, wantCalls = 1, len(steps)
			}
			python, err := exec.LookPath("python3")
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(steps)
			if err != nil {
				t.Fatal(err)
			}
			binary, _, record := fakeBinary(t, string(data))
			setNightlyTestPrerequisites(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			process := exec.CommandContext(ctx, python, "-B", "../.github/scripts/azure_integration.py",
				"check", "--binary", filepath.Join(filepath.Dir(binary), testutil.ExecutableName("az")))
			output, err := process.CombinedOutput()
			if process.ProcessState == nil {
				t.Fatalf("start script: %v", err)
			}
			code := process.ProcessState.ExitCode()
			if code != wantCode || ctx.Err() != nil || (wantCode == 0 && err != nil) {
				t.Fatalf("nightly script: code=%d output=%q error=%v", code, output, err)
			}
			if strings.Contains(string(output), "PASS:") != (wantCode == 0) {
				t.Fatalf("dishonest acceptance: %q", output)
			}
			assertFixtureCallCount(t, record, wantCalls)
		})
	}
}

func setNightlyTestPrerequisites(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"AZCTX_INTEGRATION_ENABLED":          binaryBoolTrue,
		"AZCTX_INTEGRATION_DEDICATED_TENANT": binaryBoolTrue,
		"AZCTX_TEST_CLIENT_ID":               secondTenantID,
		"AZCTX_TEST_TENANT_ID":               tenantID,
		"AZCTX_TEST_SUBSCRIPTION_ID":         productionID,
		"AZCTX_TEST_SECOND_SUBSCRIPTION_ID":  developmentID,
	} {
		t.Setenv(key, value)
	}
}

func nightlyScriptFixtures(t *testing.T) []map[string]any {
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
		{fixtureArgs: []string{listCommand, outputFlag, jsonFormat}, fixtureStdout: string(list)},
		{fixtureArgs: []string{currentFlag, outputFlag, jsonFormat}, fixtureStdout: string(current)},
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
