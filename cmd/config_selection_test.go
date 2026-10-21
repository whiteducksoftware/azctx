package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	selectionDecline = "n\n"
	selectionAlias   = "dev"
)

func writeSelectionConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := testutil.WorkDir(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"+contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigurationAndAliasCommandsWithoutAzure(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	deps.Runner, deps.FS = nil, nil
	path := writeSelectionConfig(t, "picker: fzf\nupdateCheck: false\n")
	if out := configCommandOutput(t, deps, "config", "path"); out != path+"\n" {
		t.Fatalf("config path = %q", out)
	}
	if out := configCommandOutput(t, deps, "alias", "set", "prod", productionID); out != "" {
		t.Fatalf("alias set = %q", out)
	}
	if out := configCommandOutput(t, deps, "alias", listCommand); !strings.Contains(out, "prod") || !strings.Contains(out, productionID) {
		t.Fatalf("alias list = %q", out)
	}
	if out := configCommandOutput(t, deps, "config"); !strings.Contains(out, "picker: fzf") || !strings.Contains(out, "updateCheck: false") || !strings.Contains(out, "prod:") {
		t.Fatalf("effective config = %q", out)
	}
	configCommandOutput(t, deps, "alias", "rm", "PROD")
	got, err := config.Load(deps.ConfigDir)
	if err != nil || len(got.Aliases) != 0 || got.Picker != "fzf" || got.UpdateCheck {
		t.Fatalf("unrelated settings not preserved: %+v, %v", got, err)
	}
}

func configCommandOutput(t *testing.T, deps cmd.Dependencies, args ...string) string {
	t.Helper()
	var out, messages bytes.Buffer
	status := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
	if status != 0 || messages.Len() != 0 {
		t.Fatalf("%v: status=%d, messages=%q", args, status, messages.String())
	}
	return out.String()
}

func TestProtectedSwitchConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		args        []string
		terminal    bool
		wantStatus  int
		calls       int
	}{
		{"no input", "", []string{noInputFlag, productionID}, true, 1, 0},
		{"JSON", "", []string{outputFlag, jsonFormat, productionID}, true, 1, 0},
		{"redirected input", "", []string{productionID}, false, 1, 0},
		{"yes", "", []string{confirmFlag, productionID}, false, 0, 1},
		{"JSON yes", "", []string{confirmFlag, outputFlag, jsonFormat, productionID}, false, 0, 1},
		{"confirm", "y\n", []string{productionID}, true, 0, 1},
		{"decline", selectionDecline, []string{productionID}, true, 0, 0},
		{"interrupt", "\x03", []string{productionID}, true, 130, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			deps.StdinTTY = func() bool { return tc.terminal }
			writeSelectionConfig(t, "protected: ["+tenantID+"]\n")
			var out, messages bytes.Buffer
			status := cmd.Run(tc.args, deps, strings.NewReader(tc.input), &out, &messages)
			if status != tc.wantStatus || len(fake.Calls) != tc.calls || out.Len() != 0 {
				t.Fatalf("status=%d, out=%q, messages=%q, calls=%+v", status, out.String(), messages.String(), fake.Calls)
			}
			if tc.wantStatus == 1 && !strings.Contains(messages.String(), confirmFlag) {
				t.Fatalf("missing confirmation diagnostic: %q", messages.String())
			}
		})
	}
}

func TestHiddenPinnedRecentAndUpdateSettings(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := commandDependencies(t, fake)
	writeSelectionConfig(t, "aliases:\n  dev: "+developmentID+"\nhidden: ["+developmentID+"]\npinned: ["+developmentID+"]\nupdateCheck: false\n")
	deps.CheckUpdates = func(string, string) error {
		t.Fatal("updateCheck false must suppress all update work")
		return nil
	}
	var out, messages bytes.Buffer
	if status := cmd.Run([]string{noInputFlag, selectionAlias}, deps, strings.NewReader(""), &out, &messages); status != 2 || len(fake.Calls) != 0 {
		t.Fatalf("hidden alias status=%d, messages=%q, calls=%+v", status, messages.String(), fake.Calls)
	}
	out.Reset()
	messages.Reset()
	if status := cmd.Run([]string{metadataAllFlag, selectionAlias}, deps, strings.NewReader(""), &out, &messages); status != 0 || len(fake.Calls) != 1 {
		t.Fatalf("--all alias status=%d, messages=%q, calls=%+v", status, messages.String(), fake.Calls)
	}
	deps.Select = func(subs []profile.Subscription) (int, error) {
		if len(subs) != 2 || subs[0].Id != developmentID {
			t.Fatalf("pinned picker order = %+v", subs)
		}
		return -1, ui.ErrInterrupted
	}
	if status := cmd.Run([]string{metadataAllFlag}, deps, strings.NewReader(""), &out, &messages); status != 130 {
		t.Fatalf("picker status=%d, messages=%q", status, messages.String())
	}
	got, err := state.Recent(deps.ConfigDir)
	if err != nil || !reflect.DeepEqual(got, []string{developmentID}) {
		t.Fatalf("recent = %v, %v", got, err)
	}
}

func TestProtectedCurrentPreservesJSONDTO(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	writeSelectionConfig(t, "protected: ["+productionID+"]\n")
	var out, messages bytes.Buffer
	if status := cmd.Run([]string{currentFlag}, deps, strings.NewReader(""), &out, &messages); status != 0 || !strings.Contains(out.String(), "[protected]") {
		t.Fatalf("current marker status=%d, out=%q, messages=%q", status, out.String(), messages.String())
	}
	out.Reset()
	if status := cmd.Run([]string{currentFlag, outputFlag, jsonFormat}, deps, strings.NewReader(""), &out, &messages); status != 0 {
		t.Fatalf("JSON current status=%d", status)
	}
	assertContractGolden(t, goldenCurrent, out.String())
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || len(envelope) != 2 {
		t.Fatalf("JSON contract changed: %s, %v", out.String(), err)
	}
}

func TestRankedAmbiguityNeverAutoSwitches(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	writeSelectionConfig(t, "aliases:\n  Production: "+developmentID+"\npinned: ["+developmentID+"]\n")
	if _, err := state.RecordSelection(deps.ConfigDir, productionID, developmentID); err != nil {
		t.Fatal(err)
	}
	var out, messages bytes.Buffer
	status := cmd.Run([]string{outputFlag, jsonFormat, productionName}, deps, strings.NewReader(""), &out, &messages)
	var failure struct {
		Code       string                   `json:"code"`
		Candidates []cmd.SubscriptionOutput `json:"candidates"`
	}
	if err := json.Unmarshal(messages.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if status != 2 || failure.Code != cmd.CodeAmbiguousMatch || len(failure.Candidates) != 2 ||
		failure.Candidates[0].ID != developmentID || out.Len() != 0 || len(fake.Calls) != 0 {
		t.Fatalf("ranked ambiguity status=%d, out=%q, messages=%q, calls=%+v", status, out.String(), messages.String(), fake.Calls)
	}
}

func TestProtectionAppliesToPreviousAndPicker(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(fmt.Sprint(previous), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			writeSelectionConfig(t, "protected: ["+developmentID+"]\n")
			deps.Select = func([]profile.Subscription) (int, error) { return 1, nil }
			args := []string{}
			if previous {
				if err := state.Save(deps.ConfigDir, developmentID); err != nil {
					t.Fatal(err)
				}
				args = []string{noInputFlag, "-"}
			}
			var out, messages bytes.Buffer
			status := cmd.Run(args, deps, strings.NewReader(selectionDecline), &out, &messages)
			want := 0
			if previous {
				want = 1
			}
			if status != want || len(fake.Calls) != 0 || out.Len() != 0 {
				t.Fatalf("protected route status=%d, messages=%q, calls=%+v", status, messages.String(), fake.Calls)
			}
			recent, err := state.Recent(deps.ConfigDir)
			if err != nil || len(recent) != 0 {
				t.Fatalf("refused selection recorded as recent: %v, %v", recent, err)
			}
		})
	}
}
