package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryTenantSwitchAndHistoryIsolation(t *testing.T) {
	binary, azure, record := fakeBinary(t, `[{"args":["account","set","--subscription","`+developmentID+`"]}]`)
	path := writeSelectionConfig(t, "hidden: ["+productionID+"]\nupdateCheck: false\n")
	before, err := os.ReadFile(filepath.Join(azure, profileFile))
	if err != nil {
		t.Fatal(err)
	}
	code, out, messages := runContractBinary(t, binary, []string{tenantSubcommand, tenantID}, "")
	if code != 0 || out != "" || messages != switchFeedback(developmentName, developmentID, tenantID, tenantID) {
		t.Fatalf("tenant switch exit=%d stdout=%q stderr=%q", code, out, messages)
	}
	code, out, messages = runContractBinary(t, binary, []string{historyCommand, outputFlag, jsonFormat}, "")
	var got historyOutput
	if code != 0 || messages != "" {
		t.Fatalf("history exit=%d stdout=%q stderr=%q", code, out, messages)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got.Entries) != 1 ||
		got.Entries[0].New.ID != developmentID || got.Entries[0].Source != "script" {
		t.Fatalf("binary history=%s err=%v", out, err)
	}
	disk, err := os.ReadFile(filepath.Join(filepath.Dir(path), "history.json"))
	if err != nil || string(disk) != out {
		t.Fatalf("private history=%q err=%v", disk, err)
	}
	assertHistoryAzureUnchanged(t, azure, record, before)
}

func assertHistoryAzureUnchanged(t *testing.T, azure, record string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(azure, profileFile))
	if err != nil || string(before) != string(after) {
		t.Fatalf("Azure profile changed: %v", err)
	}
	calls, err := os.ReadFile(record)
	if err != nil || strings.Count(string(calls), "\n") != 1 {
		t.Fatalf("history unexpectedly executed Azure: %s err=%v", calls, err)
	}
}
