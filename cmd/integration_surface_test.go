package cmd_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	binaryBrokerStatus = "status"
	binaryAccountClear = "clear"
	binaryBoolTrue     = "true"
	binaryMacBrokerEnv = "AZURE_CORE_ENABLE_BROKER_ON_MAC"
	binaryWinBrokerEnv = "AZURE_CORE_ENABLE_BROKER_ON_WINDOWS"
	binaryAliasCommand = "alias"
	binaryAliasHeader  = "NAME"
)

func binaryBrokerFixtures() []map[string]any {
	steps := []map[string]any{
		{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
		{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
		{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
	}
	key := brokerWindows
	if runtime.GOOS == platformMac {
		key = brokerMac
	}
	if runtime.GOOS != platformLinux {
		for _, value := range []string{binaryBoolTrue, boolFalse} {
			steps = append(steps,
				map[string]any{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
				map[string]any{fixtureArgs: []string{accountCommand, binaryAccountClear}},
				map[string]any{fixtureArgs: []string{azureConfigFile, setCommand, "core." + key + "=" + value}},
				map[string]any{fixtureArgs: []string{loginCommand}, fixtureStdout: "fake broker login\n"},
			)
		}
	}
	return steps
}

func TestBinaryDiagnosticsAndBroker(t *testing.T) {
	steps := binaryBrokerFixtures()
	data, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	binary, azure, record := fakeBinary(t, string(data))
	clearBinaryBrokerEnvironment(t)
	before := snapshotContextFiles(t, azure)
	assertBinaryDoctor(t, binary)
	code, out, messages := runContractBinary(t, binary, []string{authCommand, brokerCommand, binaryBrokerStatus}, "")
	if code != 0 || messages != "" || !strings.Contains(out, "Authentication broker:") {
		t.Fatalf("broker status code=%d stdout=%q stderr=%q", code, out, messages)
	}
	for _, action := range []string{brokerEnable, brokerDisable} {
		code, out, messages = runContractBinary(t, binary, []string{authCommand, brokerCommand, action, confirmFlag}, "")
		if runtime.GOOS == platformLinux {
			if code != 1 || out != "" || !strings.Contains(messages, "not supported") {
				t.Fatalf("unsupported broker code=%d stdout=%q stderr=%q", code, out, messages)
			}
		} else if code != 0 || out != "fake broker login\n" || !strings.Contains(messages, "clears Azure CLI accounts") {
			t.Fatalf("confirmed fake broker code=%d stdout=%q stderr=%q", code, out, messages)
		}
	}
	assertFixtureCallCount(t, record, len(steps))
	if after := snapshotContextFiles(t, azure); !maps.Equal(before, after) {
		t.Fatal("azctx directly changed Azure files")
	}
}

func clearBinaryBrokerEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{binaryWinBrokerEnv, binaryMacBrokerEnv} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBinaryDoctor(t *testing.T, binary string) {
	t.Helper()
	code, out, messages := runContractBinary(t, binary, []string{doctorCommand, outputFlag, jsonFormat}, "")
	var report cmd.DoctorReport
	if err := json.Unmarshal([]byte(out), &report); err != nil || code != 0 || messages != "" ||
		report.SchemaVersion != 1 || report.Profile.Status != "ok" || report.Profile.Subscriptions != 3 ||
		report.AzureCLI.Version != "2.91.0" {
		t.Fatalf("doctor code=%d stdout=%q stderr=%q error=%v", code, out, messages, err)
	}
	code, out, messages = runContractBinary(t, binary, []string{doctorCommand}, "")
	if code != 0 || messages != "" || !strings.Contains(out, "2.91.0") {
		t.Fatalf("doctor text code=%d stdout=%q stderr=%q", code, out, messages)
	}
}

func TestBinaryConfigAliasRoundTrip(t *testing.T) {
	binary, azure, record := fakeBinary(t, "[]")
	path := writeSelectionConfig(t, "updateCheck: false\n")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{azureConfigFile}, "version: 1"},
		{[]string{azureConfigFile, "path"}, path},
		{[]string{binaryAliasCommand, setCommand, selectionAlias, developmentID}, ""},
		{[]string{binaryAliasCommand, listCommand}, developmentID},
		{[]string{binaryAliasCommand, "rm", selectionAlias}, ""},
		{[]string{binaryAliasCommand, listCommand}, binaryAliasHeader},
	} {
		code, out, messages := runContractBinary(t, binary, tc.args, "")
		if code != 0 || messages != "" || !strings.Contains(out, tc.want) {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", tc.args, code, out, messages)
		}
		if tc.args[0] == binaryAliasCommand && tc.want == binaryAliasHeader && strings.Contains(out, developmentID) {
			t.Fatal("removed alias persisted")
		}
	}
	writeFixture(t, path, "version: 1\nunknown: true\n")
	code, out, messages := runContractBinary(t, binary, []string{azureConfigFile}, "")
	if code != 1 || out != "" || !strings.Contains(messages, "unknown key") {
		t.Fatalf("invalid config code=%d stdout=%q stderr=%q", code, out, messages)
	}
	if calls, err := os.ReadFile(record); !os.IsNotExist(err) {
		t.Fatalf("config/alias unexpectedly invoked az: %q, %v", calls, err)
	}
	if data, err := os.ReadFile(filepath.Join(azure, profileFile)); err != nil || string(data) != fixtureProfile {
		t.Fatalf("Azure profile changed: %q, %v", data, err)
	}
}

func assertFixtureCallCount(t *testing.T, record string, want int) {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil || strings.Count(string(data), "\n") != want {
		t.Fatalf("fake az calls=%q error=%v; want %d calls", data, err, want)
	}
}

func TestBinaryExecRawStreamsAndStatus(t *testing.T) {
	wantOut, wantErr := "stdout password=synthetic-only\n\x00", "stderr password=synthetic-only\n\x00"
	fixtures, err := json.Marshal([]map[string]any{{
		fixtureArgs:   []string{"literal;$(not-a-shell)", outputFlag + "=" + jsonFormat},
		fixtureStdout: wantOut, fixtureStderr: wantErr, "exitCode": 37,
	}})
	if err != nil {
		t.Fatal(err)
	}
	binary, azure, record := fakeBinary(t, string(fixtures))
	configPath := writeSelectionConfig(t, "updateCheck: false\n")
	azureBefore := snapshotContextFiles(t, azure)
	privateBefore := snapshotContextFiles(t, filepath.Dir(configPath))
	helper := filepath.Join(filepath.Dir(binary), testutil.ExecutableName("az"))
	code, out, messages := runContractBinary(t, binary, []string{
		outputFlag, jsonFormat, execSubcommand, developmentID, argumentDelimiter,
		helper, "literal;$(not-a-shell)", outputFlag + "=" + jsonFormat,
	}, "")
	if code != 37 || out != wantOut || messages != wantErr {
		t.Fatalf("raw child streams/status changed: code=%d stdout=%q stderr=%q", code, out, messages)
	}
	assertFixtureCallCount(t, record, 1)
	if !maps.Equal(azureBefore, snapshotContextFiles(t, azure)) ||
		!maps.Equal(privateBefore, snapshotContextFiles(t, filepath.Dir(configPath))) {
		t.Fatal("exec mutated Azure files or azctx config/state/history")
	}
}
