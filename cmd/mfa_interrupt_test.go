package cmd_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAzureChildInterruptionStopsMFALogin(t *testing.T) {
	for _, stage := range []string{"initial sign-in", "tenant retry", "tenant discovery"} {
		t.Run(stage, func(t *testing.T) {
			args := []string{loginCommand}
			responses := []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}}
			switch stage {
			case "tenant retry":
				responses = append(responses, testutil.Response{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}})
			case "tenant discovery":
				args = append(args, allTenantsFlag)
				responses = append(responses, testutil.Response{})
			}
			responses = append(responses, testutil.Response{Result: azcli.Result{ExitCode: 130}, Err: errors.New("child interrupted")})
			fake := &testutil.FakeRunner{Responses: responses}
			deps := commandDependencies(t, fake)
			var stdout, stderr bytes.Buffer
			if code := cmd.Run(args, deps, strings.NewReader("y\n"), &stdout, &stderr); code != 130 ||
				len(fake.Calls) != len(responses) || strings.Contains(stderr.String(), "✓ Signed in:") {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}
