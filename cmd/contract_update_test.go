package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractUpdateGates(t *testing.T) {
	const gateEnabled = "enabled"
	for _, gate := range []string{"stdout pipe", envCI, gateEmptyCI, "optout", jsonFormat, gateEnabled} {
		t.Run(gate, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			checks := 0
			deps.CheckUpdates = func(string, string) error { checks++; return nil }
			args := []string{productionName}
			switch gate {
			case "stdout pipe":
				deps.StdoutTTY = func() bool { return false }
			case envCI:
				t.Setenv(envCI, "1")
			case gateEmptyCI:
				t.Setenv(envCI, "")
			case "optout":
				t.Setenv("AZCTX_NO_UPDATE_CHECK", "1")
			case jsonFormat:
				args = append([]string{outputFlag, jsonFormat}, args...)
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || len(fake.Calls) != 1 || stdout.Len() != 0 || stderr.Len() != 0 ||
				gate == gateEnabled && checks != 1 || gate != gateEnabled && checks != 0 {
				t.Fatalf("code=%d checks=%d stdout=%q stderr=%q calls=%+v", code, checks, stdout.String(), stderr.String(), fake.Calls)
			}
		})
	}
}
