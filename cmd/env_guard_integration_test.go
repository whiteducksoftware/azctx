package cmd_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBinaryEnvironmentExecGuardNeverMutateContext(t *testing.T) {
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, testutil.ExecutableName(childFixtureName))
	testutil.Build(t, helper, childFixturePackage)
	binary, azure, calls := fakeBinary(t, "[]")
	configPath := writeSelectionConfig(t, "aliases:\n  dev: "+developmentID+"\nprotected: ["+tenantID+"]\n")
	private := filepath.Dir(configPath)
	writeFixture(t, filepath.Join(private, "state.json"), "corrupt-state-must-not-be-read-or-written\n")
	writeFixture(t, filepath.Join(private, "history.json"), "corrupt-history-must-not-be-read-or-written\n")
	writeFixture(t, filepath.Join(azure, "config"), "[cloud]\nname = AzureCloud\n")
	writeFixture(t, filepath.Join(azure, "clouds.config"), "[AzureCloud]\nsubscription = "+productionID+"\n")
	projectDir := testutil.WorkDir(t)
	writeFixture(t, filepath.Join(projectDir, projectFile), projectYAML(developmentID, secondTenantID))
	github := filepath.Join(dir, envPipelineFile)
	writeFixture(t, github, "EXISTING=value\n")
	t.Setenv("GITHUB_ENV", github)
	t.Setenv(childModeEnv, "echo")
	azureBefore, privateBefore := snapshotContextFiles(t, azure), snapshotContextFiles(t, private)
	guardGolden, err := os.ReadFile(filepath.Join("testdata", "contract", "guard-mismatch.golden"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{envCommand}, 0},
		{[]string{envCommand, selectionAlias, shellFlag, envZshSyntax}, 0},
		{[]string{envCommand, shellFlag, envFishSyntax}, 0},
		{[]string{envCommand, shellFlag, envPSSyntax}, 0},
		{[]string{envCommand, formatFlag, envGitHubFormat}, 0},
		{[]string{envCommand, formatFlag, envDevOpsFormat}, 0},
		{[]string{execSubcommand, selectionAlias, argumentDelimiter, helper, outputFlag + "=" + jsonFormat, "literal;$(bad)\nvalue"}, 0},
		{[]string{guardCommand, subscriptionFlag, productionID, tenantFlag, tenantID}, 0},
		{[]string{guardCommand, outputFlag, jsonFormat}, 4},
		{[]string{currentFlag, outputFlag, idFormat}, 0},
		{[]string{currentFlag}, 0},
		{[]string{listCommand}, 0},
	} {
		code, out, messages := runContractBinary(t, binary, tc.args, "child stdin\n")
		if code != tc.code {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", tc.args, code, out, messages)
		}
		assertContextProcessResult(t, tc.args[0], tc.code, out, messages, string(guardGolden))
	}
	// A malformed cloned file is an explicit guard error, never a login or switch.
	writeFixture(t, filepath.Join(projectDir, projectFile), "version: 1\nlogin: true\n")
	code, _, messages := runContractBinary(t, binary, []string{guardCommand}, "")
	if code != 1 || !strings.Contains(messages, "unknown key") {
		t.Fatalf("malformed guard: %d %q", code, messages)
	}
	// Even an explicitly requested pipeline output cannot write Azure config.
	t.Setenv("GITHUB_ENV", filepath.Join(azure, "config"))
	code, out, messages := runContractBinary(t, binary, []string{envCommand, formatFlag, envGitHubFormat}, "")
	if code != 1 || out != "" || !strings.Contains(messages, "Azure configuration") {
		t.Fatalf("unsafe pipeline output: %d %q %q", code, out, messages)
	}
	if !reflect.DeepEqual(azureBefore, snapshotContextFiles(t, azure)) || !reflect.DeepEqual(privateBefore, snapshotContextFiles(t, private)) {
		t.Fatal("read-only commands mutated Azure configuration or azctx config/state/history/cache")
	}
	if data, err := os.ReadFile(calls); !os.IsNotExist(err) {
		t.Fatalf("read-only commands invoked Azure CLI: %q, %v", data, err)
	}
}

func snapshotContextFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	result := map[string]string{}
	err = fs.WalkDir(files.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertContextProcessResult(t *testing.T, command string, code int, out, messages, guardGolden string) {
	t.Helper()
	if command == execSubcommand {
		var result struct {
			Env map[string]string `json:"env"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || result.Env["ARM_SUBSCRIPTION_ID"] != developmentID || messages != childStderr {
			t.Fatalf("child context=%q stderr=%q error=%v", out, messages, err)
		}
	} else if code == 4 && messages != guardGolden {
		t.Fatalf("guard error contract: %q", messages)
	}
}
