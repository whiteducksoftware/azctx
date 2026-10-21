package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/log"
)

const fixtureProfile = `{"subscriptions":[{"id":"prod-id","name":"Production","tenantId":"tenant","isDefault":true},{"id":"dev-id","name":"Development","tenantId":"tenant"},{"id":"tenant","name":"N/A(tenant level account)"}]}`

const argumentDelimiter = "--"

const productionName = "Production"

const deviceCodeFlag = "--use-device-code"

const loginCommand = "login"

func commandDependencies(t *testing.T, fake *testutil.FakeRunner) cmd.Dependencies {
	t.Helper()
	t.Setenv("PATH", "")
	files := afero.NewMemMapFs()
	if err := files.MkdirAll("config", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(files, filepath.Join("config", "azureProfile.json"), []byte(fixtureProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	return cmd.Dependencies{
		Runner: fake, FS: files, ConfigDir: "config",
		CheckUpdates: func(string, string) error { return nil },
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
		{name: "name", args: []string{argumentDelimiter, productionName}, want: "prod-id"},
		{name: "picker", want: "dev-id"},
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
			contents, err := afero.ReadFile(deps.FS, filepath.Join("config", "azureProfile.json"))
			if err != nil || string(contents) != fixtureProfile {
				t.Fatalf("profile changed: %q, %v", contents, err)
			}
		})
	}
}

func TestCurrentSubscription(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	var output bytes.Buffer
	log.InitLogger(&output)
	t.Cleanup(func() { log.InitLogger(os.Stderr) })
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{"-c"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 0 || !strings.Contains(output.String(), "Active subscription: /Production (tenant/prod-id)") {
		t.Fatalf("current output = %q, az calls = %+v", output.String(), fake.Calls)
	}
}

func TestCommandsReturnErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{argumentDelimiter, productionName}, want: "Error executing command: denied"},
		{args: []string{argumentDelimiter, "missing"}, want: "no azure subscription found for 'missing'"},
		{args: []string{productionName}, want: `unknown command "Production" for "azctx"`},
		{args: []string{"-"}, want: "not implemented"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{
				Result: azcli.Result{Stderr: "denied", ExitCode: 1}, Err: errors.New("exit status 1"),
			}}}
			root := cmd.NewRootCommand(commandDependencies(t, fake))
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil || err.Error() != tc.want {
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
	root.SetArgs([]string{loginCommand, argumentDelimiter, deviceCodeFlag, "--tenant", "tenant"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"rest", "--method", "get", "--url", "/tenants?api-version=2020-01-01", "--output", "json"},
		{loginCommand, deviceCodeFlag, "--tenant", "tenant"},
	}
	var got [][]string
	for _, call := range fake.Calls {
		got = append(got, call.Args)
	}
	if !reflect.DeepEqual(got, want) || fake.Calls[1].Attached {
		t.Fatalf("az calls = %+v", fake.Calls)
	}
}

func TestCommandReturnsUpdateError(t *testing.T) {
	failure := errors.New("offline")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := commandDependencies(t, fake)
	deps.FS = afero.NewReadOnlyFs(deps.FS)
	deps.CheckUpdates = func(string, string) error { return failure }
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{argumentDelimiter, productionName})
	err := root.Execute()
	var handlerErr *cmd.HandlerError
	if !errors.Is(err, failure) || !errors.As(err, &handlerErr) || len(fake.Calls) != 1 {
		t.Fatalf("post-switch update error = %v, calls = %+v", err, fake.Calls)
	}
}

func TestCanceledPickerAndIndependentFlags(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	deps.Select = func(subscriptions []profile.Subscription) (int, error) {
		if len(subscriptions) != 3 {
			t.Fatalf("unfiltered subscriptions = %+v", subscriptions)
		}
		return -1, errors.New("canceled")
	}
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{"--filter-tenant-level=false"})
	if err := root.Execute(); err != nil || len(fake.Calls) != 0 {
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
