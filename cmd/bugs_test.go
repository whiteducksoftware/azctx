package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestDuplicateNamesKeepTenantAndIDAfterPickerSort(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := commandDependencies(t, fake)
	deps.FS = fstest.MapFS{
		profileFile: &fstest.MapFile{Data: []byte(`{"subscriptions":[
			{"id":"` + productionID + `","name":"Visual Studio Enterprise","tenantId":"` + tenantID + `","tenantDisplayName":"Zulu","environmentName":"AzureCloud","isDefault":true},
			{"id":"` + developmentID + `","name":"Visual Studio Enterprise","tenantId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","tenantDisplayName":"Acme","environmentName":"AzureCloud"}
		]}`)},
	}
	deps.Select = func(subscriptions []profile.Subscription) (int, error) {
		if len(subscriptions) != 2 {
			t.Fatalf("duplicate-name matches=%+v", subscriptions)
		}
		picker := ui.NewPicker(subscriptions, 160, 20)
		view := picker.View().Content
		if !strings.Contains(view, "Acme") || !strings.Contains(view, "Zulu") {
			t.Fatalf("tenant names missing from picker: %s", view)
		}
		return 0, nil
	}
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{argumentDelimiter, "Visual Studio Enterprise"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].Args[3] != developmentID {
		t.Fatalf("wrong duplicate selected: %+v", fake.Calls)
	}
}

func TestVerboseOnlyEnablesDebugDiagnostics(t *testing.T) {
	for _, verbose := range []bool{false, true, false} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
		deps := commandDependencies(t, fake)
		deps.CheckUpdates = func(string, string) error { return errors.New("offline diagnostic") }
		root := cmd.NewRootCommand(deps)
		args := []string{argumentDelimiter, productionName}
		if verbose {
			args = append([]string{"--verbose"}, args...)
		}
		root.SetArgs(args)
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if stdout.Len() != 0 || strings.Contains(stderr.String(), "offline diagnostic") != verbose ||
			strings.Contains(stderr.String(), "Setting active subscription") != verbose {
			t.Fatalf("verbose=%v stdout=%q stderr=%q", verbose, stdout.String(), stderr.String())
		}
	}
}

func TestPickerErrorsNeverSwitchOrCheckUpdates(t *testing.T) {
	for _, query := range [][]string{nil, {argumentDelimiter, "Production"}} {
		for _, invalidIndex := range []bool{false, true} {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			// Two real IDs force both the full and narrowed picker paths.
			deps.FS = fstest.MapFS{
				profileFile: &fstest.MapFile{Data: []byte(strings.ReplaceAll(fixtureProfile, "Development", "Production"))},
			}
			failure := errors.New("terminal failed")
			deps.Select = func([]profile.Subscription) (int, error) {
				if invalidIndex {
					return 100, nil
				}
				return -1, failure
			}
			deps.CheckUpdates = func(string, string) error {
				t.Error("failed picker triggered update check")
				return nil
			}
			root := cmd.NewRootCommand(deps)
			root.SetArgs(query)
			err := root.Execute()
			if err == nil || !invalidIndex && !errors.Is(err, failure) || len(fake.Calls) != 0 {
				t.Fatalf("picker error=%v calls=%+v", err, fake.Calls)
			}
		}
	}
}

func TestMissingAndMalformedProfilePreserveCauses(t *testing.T) {
	for _, data := range []string{"", "{"} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		files := fstest.MapFS{}
		if data != "" {
			files[profileFile] = &fstest.MapFile{Data: []byte(data)}
		}
		deps.FS = files
		root := cmd.NewRootCommand(deps)
		root.SetArgs([]string{currentFlag})
		err := root.Execute()
		var syntax *json.SyntaxError
		if data == "" && (!errors.Is(err, fs.ErrNotExist) || !strings.Contains(fmt.Sprint(err), "az login")) ||
			data != "" && !errors.As(err, &syntax) {
			t.Fatalf("profile %q error=%v", data, err)
		}
	}
}

func TestPreviousStateValidationPreventsAZ(t *testing.T) {
	for _, data := range []string{
		`{"previousSubscriptionId":"99999999-9999-9999-9999-999999999999"}`,
		`{"previousSubscriptionId":"Production"}`, `{`,
	} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		config, err := os.UserConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(config, "azctx")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(dir, "state.json"), data)
		root := cmd.NewRootCommand(deps)
		root.SetArgs([]string{"-"})
		if err := root.Execute(); err == nil || len(fake.Calls) != 0 {
			t.Fatalf("previous state=%s error=%v calls=%+v", data, err, fake.Calls)
		}
	}
}

func TestFailedSwitchPreservesPreviousState(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			failure := errors.New("switch denied")
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Err: failure}}}
			deps := commandDependencies(t, fake)
			if existing {
				if err := state.Save(deps.ConfigDir, tenantID); err != nil {
					t.Fatal(err)
				}
			}
			root := cmd.NewRootCommand(deps)
			root.SetArgs([]string{argumentDelimiter, "Development"})
			if err := root.Execute(); !errors.Is(err, failure) {
				t.Fatalf("switch failure=%v", err)
			}
			previous, err := state.Load(deps.ConfigDir)
			if existing && (err != nil || previous != tenantID) || !existing && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("failed switch changed previous=%q error=%v", previous, err)
			}
		})
	}
}

func TestSwitchPreservesRunnerError(t *testing.T) {
	failure := errors.New("process failure")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stderr: "denied"}, Err: failure}}}
	root := cmd.NewRootCommand(commandDependencies(t, fake))
	root.SetArgs([]string{argumentDelimiter, productionName})
	if err := root.Execute(); !errors.Is(err, failure) {
		t.Fatalf("lost runner error: %v", err)
	}
}
