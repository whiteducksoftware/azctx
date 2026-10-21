package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type canceledLoginRunner struct {
	fake   *testutil.FakeRunner
	cancel context.CancelFunc
	atCall int
}

func (r canceledLoginRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	result, err := r.fake.Run(ctx, args, options)
	if len(r.fake.Calls) == r.atCall {
		r.cancel()
	}
	return result, err
}

func TestLoginPreservesCancellationAtEveryStage(t *testing.T) {
	const allTenantStage = "all tenant"
	for _, stage := range []string{"version", "initial", "retry", "discovery", allTenantStage} {
		t.Run(stage, func(t *testing.T) {
			responses := []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {}}
			args, atCall := []string{loginCommand}, 1
			switch stage {
			case "initial":
				atCall = 2
			case "retry":
				responses[1].Result.Stderr = mfaFixture(t, "50076")
				responses = append(responses, testutil.Response{})
				atCall = 3
			case "discovery", allTenantStage:
				args = append(args, allTenantsFlag)
				responses = append(responses, testutil.Response{Result: azcli.Result{Stdout: `[{"tenantId":"` + mfaTenant + `"}]`}}, testutil.Response{})
				atCall = 3
				if stage == allTenantStage {
					atCall = 4
				}
			}
			fake := &testutil.FakeRunner{Responses: responses}
			deps := commandDependencies(t, fake)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deps.Runner = canceledLoginRunner{fake: fake, cancel: cancel, atCall: atCall}
			command := cmd.NewRootCommand(deps)
			command.SetContext(ctx)
			command.SetArgs(args)
			command.SetIn(strings.NewReader("y\n"))
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			err := command.Execute()
			if cmd.ExitCode(err) != 130 || !errors.Is(err, context.Canceled) || len(fake.Calls) != atCall ||
				strings.Contains(stderr.String(), "Successfully fetched") {
				t.Fatalf("error=%v calls=%+v stdout=%q stderr=%q", err, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}
