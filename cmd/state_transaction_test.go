package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

// switchRunner observes the private state while az account set runs.
type switchRunner struct {
	fake   *testutil.FakeRunner
	during func(context.Context)
}

func (r switchRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	if len(args) >= 2 && slices.Equal(args[:2], []string{accountCommand, setCommand}) {
		r.during(ctx)
	}
	result, err := r.fake.Run(ctx, args, options)
	if ctx.Err() != nil {
		// Like the production runner, an interrupted az keeps the cancellation cause.
		err = errors.Join(ctx.Err(), err)
	}
	return result, err
}

func stateLockAvailable(t *testing.T, private string) bool {
	t.Helper()
	release, acquired, err := storage.Lock(filepath.Join(private, "state.lock"), azureConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		release()
	}
	return acquired
}

func TestSwitchRetainsStateLockThroughAzureCall(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
	deps := commandDependencies(t, fake)
	private := filepath.Dir(writeSelectionConfig(t, ""))
	held := false
	deps.Runner = switchRunner{fake: fake, during: func(context.Context) { held = !stateLockAvailable(t, private) }}
	var out, messages bytes.Buffer
	if status := cmd.Run([]string{developmentID}, deps, strings.NewReader(""), &out, &messages); status != 0 || len(fake.Calls) != 1 {
		t.Fatalf("status=%d stderr=%q calls=%+v", status, &messages, fake.Calls)
	}
	if !held {
		t.Fatal("az account set ran without the retained state lock")
	}
	if !stateLockAvailable(t, private) {
		t.Fatal("completed switch kept the state lock")
	}
	if got := configCommandOutput(t, deps, historyCommand, outputFlag, jsonFormat); !strings.Contains(got, developmentID) {
		t.Fatalf("completed switch was not recorded: %q", got)
	}
}

func TestFailedSwitchRollbackKeepsUnlockedWriterUpdate(t *testing.T) {
	// An older azctx without state.lock may commit while az account set runs.
	const foreign = "{\"previousSubscriptionId\":\"" + productionID + "\"}"
	for _, tc := range []struct {
		name   string
		cancel bool
		status int
	}{
		{"failure", false, 1},
		{"canceled", true, 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{ExitCode: 1, Stderr: "set failed"}, Err: errors.New("exit status 1")}}}
			deps := commandDependencies(t, fake)
			private := filepath.Dir(writeSelectionConfig(t, ""))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deps.Runner = switchRunner{fake: fake, during: func(context.Context) {
				writeFixture(t, filepath.Join(private, "state.json"), foreign)
				if tc.cancel {
					cancel()
				}
			}}
			root := cmd.NewRootCommand(deps)
			root.SetArgs([]string{developmentID})
			var out, messages bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&messages)
			err := root.ExecuteContext(ctx)
			if cmd.ExitCode(err) != tc.status || out.Len() != 0 || !strings.Contains(err.Error(), "set failed") ||
				!strings.Contains(err.Error(), "changed by another process") {
				t.Fatalf("error=%v stdout=%q stderr=%q", err, &out, &messages)
			}
			if data, err := os.ReadFile(filepath.Join(private, "state.json")); err != nil || string(data) != foreign {
				t.Fatalf("rollback erased another writer's state: %q, %v", data, err)
			}
			if !stateLockAvailable(t, private) {
				t.Fatal("failed switch kept the state lock")
			}
		})
	}
}
