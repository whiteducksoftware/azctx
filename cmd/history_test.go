package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type historyIdentity struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenantId"`
	Environment string `json:"environment"`
}

const historyCommand = "history"

func TestHistoryRecoveryAndWriteFailureNeverFailSwitch(t *testing.T) {
	const corruptHistory = "corrupt"
	for _, broken := range []string{corruptHistory, "directory"} {
		t.Run(broken, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			configPath := writeSelectionConfig(t, "")
			path := filepath.Join(filepath.Dir(configPath), "history.json")
			if broken == corruptHistory {
				if err := os.WriteFile(path, []byte("{secret garbage"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			var out, messages bytes.Buffer
			if code := cmd.Run([]string{developmentID, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages); code != 0 ||
				len(fake.Calls) != 1 || out.Len() != 0 || !strings.HasPrefix(messages.String(), "Warning: ") ||
				strings.Contains(messages.String(), "secret") || strings.Count(messages.String(), "\n") != 1 {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, &out, &messages)
			}
			if broken == corruptHistory {
				messages.Reset()
				if code := cmd.Run([]string{historyCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages); code != 0 || messages.Len() != 0 ||
					!strings.Contains(out.String(), developmentID) {
					t.Fatalf("recovery=%d stdout=%q stderr=%q", code, &out, &messages)
				}
			}
		})
	}
}

func TestHistoryReadOnlyWithoutProfileAndCorruptionWarning(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	dir := testutil.WorkDir(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	deps.FS, deps.Runner = nil, nil
	out := configCommandOutput(t, deps, historyCommand, outputFlag, jsonFormat)
	if out != emptyHistoryJSON {
		t.Fatalf("empty history=%q", out)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
		t.Fatalf("read-only history wrote files: %v %v", files, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("corrupt secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{historyCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &stdout, &stderr); code != 0 ||
		stdout.String() != out || stderr.String() != "Warning: corrupt history ignored.\n" {
		t.Fatalf("corrupt read=%d stdout=%q stderr=%q", code, &stdout, &stderr)
	}
}

func TestHistoryTTYNoPreviousAndHiddenIndependence(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	subs := tenantSubscriptions()
	subs[0].IsDefault = false
	deps := tenantDependencies(t, fake, subs)
	writeSelectionConfig(t, "hidden: ["+productionID+"]\n")
	var out, messages bytes.Buffer
	if code := cmd.Run([]string{tenantSubcommand, tenantID, metadataAllFlag, noInputFlag}, deps, strings.NewReader(""), &out, &messages); code != 0 {
		t.Fatalf("TTY switch=%d stderr=%q", code, &messages)
	}
	deps.FS, deps.Runner = nil, nil
	deps.Select = func([]profile.Subscription) (int, error) { t.Fatal("history must not select"); return -1, nil }
	if code := cmd.Run([]string{historyCommand, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages); code != 0 {
		t.Fatalf("history=%d stderr=%q", code, &messages)
	}
	var got historyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got.Entries) != 1 || got.Entries[0].Previous != nil ||
		got.Entries[0].Source != "tty" || got.Entries[0].New.ID != productionID {
		t.Fatalf("history=%s err=%v", &out, err)
	}
}

type historyOutput struct {
	SchemaVersion int `json:"schemaVersion"`
	Entries       []struct {
		Timestamp time.Time        `json:"timestamp"`
		Previous  *historyIdentity `json:"previous"`
		New       historyIdentity  `json:"new"`
		Source    string           `json:"source"`
	} `json:"entries"`
}

func TestHistoryRecordsOnlyCompletedSwitches(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}, {Err: errors.New("synthetic switch failure")}}}
	deps := commandDependencies(t, fake)
	configPath := writeSelectionConfig(t, "")
	deps.StdinTTY = func() bool { return false }
	before := time.Now()
	var out, messages bytes.Buffer
	if code := cmd.Run([]string{developmentID}, deps, strings.NewReader(""), &out, &messages); code != 0 {
		t.Fatalf("switch=%d stderr=%q", code, &messages)
	}
	if code := cmd.Run([]string{productionID}, deps, strings.NewReader(""), &out, &messages); code != 1 {
		t.Fatalf("failed switch=%d", code)
	}
	out.Reset()
	messages.Reset()
	if code := cmd.Run([]string{historyCommand, outputFlag, jsonFormat, verboseFlag}, deps, strings.NewReader(""), &out, &messages); code != 0 || messages.Len() != 0 {
		t.Fatalf("history=%d stdout=%q stderr=%q", code, &out, &messages)
	}
	var got historyOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || len(got.Entries) != 1 {
		t.Fatalf("history=%s", &out)
	}
	assertScriptHistoryEntry(t, got, before)
	if bytes.ContainsRune(out.Bytes(), '\r') || bytes.Count(out.Bytes(), []byte{'\n'}) != 1 ||
		strings.Contains(out.String(), "user") || strings.Contains(out.String(), "synthetic") {
		t.Fatalf("noisy history JSON=%q", &out)
	}
	again := configCommandOutput(t, deps, historyCommand, outputFlag, jsonFormat)
	if again != out.String() {
		t.Fatalf("unstable history: %q != %q", again, &out)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "history.json"))
	if err != nil || string(data) != out.String() {
		t.Fatalf("disk/CLI format differs: %q err=%v", data, err)
	}
}

func assertScriptHistoryEntry(t *testing.T, got historyOutput, before time.Time) {
	t.Helper()
	entry := got.Entries[0]
	if entry.New.ID != developmentID || entry.Previous == nil || entry.Previous.ID != productionID ||
		entry.Source != "script" || entry.New.TenantID != tenantID || entry.New.Environment != azureCloudName ||
		entry.Timestamp.Before(before) || entry.Timestamp.After(time.Now()) {
		t.Fatalf("entry=%+v", entry)
	}
}
