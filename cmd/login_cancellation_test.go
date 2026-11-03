package cmd_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type canceledLoginRunner struct {
	fake   *testutil.FakeRunner
	cancel context.CancelFunc
}

func (r canceledLoginRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	if args[0] == loginCommand {
		r.cancel()
	}
	return r.fake.Run(ctx, args, options)
}

func TestExistingIterativeLoginPreservesContextCancellation(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {Err: context.Canceled}}}
	deps := commandDependencies(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deps.Runner = canceledLoginRunner{fake, cancel}
	command := cmd.NewRootCommand(deps)
	command.SetContext(ctx)
	command.SetArgs([]string{loginCommand, forceMFAFlag})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	if err := command.Execute(); cmd.ExitCode(err) != 130 || len(fake.Calls) != 2 {
		t.Fatalf("cancellation error=%v calls=%+v stdout=%q stderr=%q", err, fake.Calls, stdout.String(), stderr.String())
	}
}
