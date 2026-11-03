package cmd_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	productionID   = "11111111-1111-1111-1111-111111111111"
	developmentID  = "22222222-2222-2222-2222-222222222222"
	tenantID       = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	fixtureProfile = `{"subscriptions":[{"id":"` + productionID + `","name":"Production","tenantId":"` + tenantID + `","isDefault":true},{"id":"` + developmentID + `","name":"Development","tenantId":"` + tenantID + `"},{"id":"` + tenantID + `","name":"N/A(tenant level account)"}]}`
)

const argumentDelimiter = "--"

const productionName = "Production"

const deviceCodeFlag = "--use-device-code"

const (
	loginCommand = "login"
	profileFile  = "azureProfile.json"
	currentFlag  = "-c"
)

func commandDependencies(t *testing.T, fake *testutil.FakeRunner) cmd.Dependencies {
	t.Helper()
	t.Setenv("PATH", "")
	testutil.IsolateUserDirs(t)
	t.Setenv("CI", "")
	if err := os.Unsetenv("CI"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZCTX_NO_INPUT", "")
	t.Setenv("AZCTX_NO_UPDATE_CHECK", "")
	files := fstest.MapFS{
		profileFile: &fstest.MapFile{Data: []byte(fixtureProfile)},
	}
	return cmd.Dependencies{
		Runner: fake, FS: files, ConfigDir: "config",
		CheckUpdates: func(string, string) error { return nil },
		StdinTTY:     func() bool { return true },
		StdoutTTY:    func() bool { return true },
		Select: func(subscriptions []profile.Subscription) (int, error) {
			if len(subscriptions) != 2 {
				t.Fatalf("picker subscriptions = %+v", subscriptions)
			}
			return 1, nil
		},
	}
}

func TestCommandSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "name", args: []string{argumentDelimiter, productionName}, want: productionID},
		{name: "picker", want: developmentID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			root := cmd.NewRootCommand(deps)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, []string{"account", "set", "--subscription", tc.want}) {
				t.Fatalf("az calls = %+v", fake.Calls)
			}
			contents, err := fs.ReadFile(deps.FS, profileFile)
			if err != nil || string(contents) != fixtureProfile {
				t.Fatalf("profile changed: %q, %v", contents, err)
			}
		})
	}
}

func TestCurrentSubscription(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	var output, messages bytes.Buffer
	root := cmd.NewRootCommand(deps)
	root.SetOut(&output)
	root.SetErr(&messages)
	root.SetArgs([]string{currentFlag})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 0 || output.String() != "Active subscription: "+tenantID+"/Production ("+tenantID+"/"+productionID+")\n" {
		t.Fatalf("current output = %q, az calls = %+v", output.String(), fake.Calls)
	}
	if strings.Contains(messages.String(), "Active subscription:") || strings.Contains(messages.String(), "level=") {
		t.Fatalf("messages = %q", messages.String())
	}
}

func TestSwitchIsQuietOnBothStreams(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	root := cmd.NewRootCommand(commandDependencies(t, fake))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{argumentDelimiter, productionName})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCommandsReturnErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{argumentDelimiter, productionName}, want: "Error executing command: denied"},
		{args: []string{argumentDelimiter, "missing"}, want: "no azure subscription found for 'missing'"},
		{args: []string{productionName}, want: "Error executing command: denied"},
		{args: []string{"-"}, want: "no previous subscription found"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{
				Result: azcli.Result{Stderr: "denied", ExitCode: 1}, Err: errors.New("exit status 1"),
			}}}
			root := cmd.NewRootCommand(commandDependencies(t, fake))
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoginPassesThroughArguments(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: `{"value":[]}`}}, {},
	}}
	root := cmd.NewRootCommand(commandDependencies(t, fake))
	root.SetArgs([]string{loginCommand, argumentDelimiter, deviceCodeFlag, "--tenant", tenantID})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"rest", "--method", "get", "--url", "/tenants?api-version=2020-01-01", "--output", jsonFormat},
		{loginCommand, deviceCodeFlag, "--tenant", tenantID},
	}
	var got [][]string
	for _, call := range fake.Calls {
		got = append(got, call.Args)
	}
	if !reflect.DeepEqual(got, want) || !fake.Calls[1].Attached {
		t.Fatalf("az calls = %+v", fake.Calls)
	}
}

func TestOfflineUpdateCannotFailSwitch(t *testing.T) {
	failure := errors.New("offline")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := commandDependencies(t, fake)
	deps.CheckUpdates = func(string, string) error { return failure }
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{argumentDelimiter, productionName})
	err := root.Execute()
	if err != nil || len(fake.Calls) != 1 {
		t.Fatalf("post-switch update error = %v, calls = %+v", err, fake.Calls)
	}
}

func TestPickerInterruptReachesExitBoundary(t *testing.T) {
	for _, args := range [][]string{nil, {argumentDelimiter, "PRD"}} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		deps.Select = func([]profile.Subscription) (int, error) { return -1, ui.ErrInterrupted }
		// Duplicate names must also propagate interruption from the narrowed picker.
		if args != nil {
			deps.FS = fstest.MapFS{
				profileFile: &fstest.MapFile{Data: []byte(`{"subscriptions":[{"id":"one","name":"Production"},{"id":"two","name":"Production"}]}`)},
			}
		}
		deps.CheckUpdates = func(string, string) error {
			t.Fatal("interruption must not trigger the update check")
			return nil
		}
		root := cmd.NewRootCommand(deps)
		root.SetArgs(args)
		if err := root.Execute(); !errors.Is(err, ui.ErrInterrupted) || len(fake.Calls) != 0 {
			t.Fatalf("interrupt = %v, az calls=%+v", err, fake.Calls)
		}
	}
}

func TestCanceledPickerAndIndependentFlags(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	deps.Select = func(subscriptions []profile.Subscription) (int, error) {
		if len(subscriptions) != 2 {
			t.Fatalf("filtered subscriptions = %+v", subscriptions)
		}
		return -1, errors.New("canceled")
	}
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{"--verbose"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "canceled") || len(fake.Calls) != 0 {
		t.Fatalf("canceled picker = %v, calls = %+v", err, fake.Calls)
	}
	// A fresh tree must not inherit the preceding tree's flag state.
	fake.Responses = []testutil.Response{{}}
	root = cmd.NewRootCommand(commandDependencies(t, fake))
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
