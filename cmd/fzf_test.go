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
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestConfiguredFZFPreservesRankingAndProtection(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("fzf")), "github.com/whiteducksoftware/azctx/internal/testutil/fakefzf")
	for _, tc := range []struct {
		name, exit, output, input string
		code, switches            int
	}{
		{"ranked", "", "", "y\n", 0, 1},
		{"decline protected", "", "", selectionDecline, 1, 0},
		{"Escape", "", "esc\n", "", 1, 0},
		{"empty result", "1", "", "", 1, 0},
		{"Ctrl+C", "130", "", "", 130, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			t.Setenv("PATH", dir)
			t.Setenv("FAKEFZF_RECORD", filepath.Join(testutil.WorkDir(t), "record.json"))
			t.Setenv("FAKEFZF_EXIT", tc.exit)
			t.Setenv("FAKEFZF_INDEX", "0")
			if tc.output != "" {
				t.Setenv("FAKEFZF_OUTPUT", tc.output)
			}
			writeSelectionConfig(t, "picker: fzf\npinned: ["+developmentID+"]\nprotected: ["+developmentID+"]\n")
			deps.Select = func([]profile.Subscription) (int, error) {
				t.Fatal("configured fzf must execute the binary")
				return -1, nil
			}
			var out, messages bytes.Buffer
			code := cmd.Run(nil, deps, strings.NewReader(tc.input), &out, &messages)
			if code != tc.code || len(fake.Calls) != tc.switches || out.Len() != 0 {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, &out, &messages)
			}
			if tc.switches != 0 && !reflect.DeepEqual(fake.Calls[0].Args, []string{accountCommand, setCommand, subscriptionFlag, developmentID}) {
				t.Fatalf("ranked identity lost: %+v", fake.Calls)
			}
			if tc.switches == 0 && configCommandOutput(t, deps, historyCommand, outputFlag, jsonFormat) != emptyHistoryJSON {
				t.Fatal("canceled/unconfirmed fzf switch was recorded")
			}
		})
	}
}

func TestMissingFZFFallbackAndInputGates(t *testing.T) {
	for _, args := range [][]string{nil, {noInputFlag}, {outputFlag, jsonFormat}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			deps := commandDependencies(t, &testutil.FakeRunner{Responses: []testutil.Response{{}}})
			writeSelectionConfig(t, "picker: fzf\n")
			selected := false
			deps.Select = func([]profile.Subscription) (int, error) { selected = true; return 0, nil }
			var out, messages bytes.Buffer
			if status := cmd.Run(args, deps, strings.NewReader(""), &out, &messages); status != 0 {
				t.Fatalf("status=%d messages=%q", status, &messages)
			}
			if len(args) == 0 {
				if !selected || messages.String() != "fzf is not on PATH; using the built-in picker.\n"+switchFeedback(productionName, productionID, tenantID, tenantID) || out.Len() != 0 {
					t.Fatalf("fallback selected=%t stdout=%q stderr=%q", selected, &out, &messages)
				}
			} else if selected || messages.Len() != 0 {
				t.Fatalf("input gate bypassed: selected=%t stderr=%q", selected, &messages)
			}
		})
	}
}

func TestTenantMissingFZFNoticesOnce(t *testing.T) {
	subs := tenantSubscriptions()
	subs = append(subs, profile.Subscription{Id: otherSubscriptionID, Name: "Other", Tenant: secondTenantID, TenantName: customerTenant, Environment: azureCloudName})
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := tenantDependencies(t, fake, subs)
	writeSelectionConfig(t, "picker: fzf\n")
	stages := 0
	deps.Select = func([]profile.Subscription) (int, error) {
		stages++
		if stages == 1 {
			return 1, nil
		}
		return 0, nil
	}
	var out, messages bytes.Buffer
	code := cmd.Run([]string{tenantSubcommand}, deps, strings.NewReader(""), &out, &messages)
	if code != 0 || stages != 2 || len(fake.Calls) != 1 || messages.String() != "fzf is not on PATH; using the built-in picker.\n"+switchFeedback(developmentName, developmentID, customerTenant, secondTenantID) {
		t.Fatalf("code=%d stages=%d calls=%+v messages=%q", code, stages, fake.Calls, &messages)
	}
}

func TestTenantFZFDoesNotConfuseDuplicateLabels(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("fzf")), "github.com/whiteducksoftware/azctx/internal/testutil/fakefzf")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := tenantDependencies(t, fake, tenantSubscriptions())
	writeSelectionConfig(t, "picker: fzf\n")
	t.Setenv("PATH", dir)
	record := filepath.Join(dir, "record.json")
	t.Setenv("FAKEFZF_RECORD", record)
	t.Setenv("FAKEFZF_EXIT", "")
	t.Setenv("FAKEFZF_INDEX", "1")
	var out, messages bytes.Buffer
	if code := cmd.Run([]string{tenantSubcommand, customerTenant}, deps, strings.NewReader(""), &out, &messages); code != 0 || len(fake.Calls) != 1 || fake.Calls[0].Args[3] != developmentID {
		t.Fatalf("code=%d calls=%+v messages=%q", code, fake.Calls, &messages)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var invocation struct{ Input string }
	if err := json.Unmarshal(data, &invocation); err != nil || !strings.Contains(invocation.Input, secondTenantID) {
		t.Fatalf("tenant rows lost GUID: %s, %v", data, err)
	}
}
