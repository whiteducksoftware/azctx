package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractUpdateGates(t *testing.T) {
	const gateEnabled = "enabled"
	for _, gate := range []string{"stdout pipe", envCI, gateEmptyCI, "optout", "config optout", jsonFormat, gateEnabled} {
		t.Run(gate, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			checks := 0
			deps.CheckUpdates = func(string, string) error { checks++; return nil }
			args := []string{versionCommand}
			switch gate {
			case "stdout pipe":
				deps.StdoutTTY = func() bool { return false }
			case envCI:
				t.Setenv(envCI, "1")
			case gateEmptyCI:
				t.Setenv(envCI, "")
			case "optout":
				t.Setenv("AZCTX_NO_UPDATE_CHECK", "1")
			case "config optout":
				cfg := config.Default()
				cfg.UpdateCheck = false
				if err := config.Save(deps.ConfigDir, cfg); err != nil {
					t.Fatal(err)
				}
			case jsonFormat:
				args = append([]string{outputFlag, jsonFormat}, args...)
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
			wantChecks := 0
			if gate == gateEnabled {
				wantChecks = 1
			}
			if code != 0 || len(fake.Calls) != 0 || stdout.Len() == 0 || stderr.Len() != 0 || checks != wantChecks {
				t.Fatalf("code=%d checks=%d stdout=%q stderr=%q calls=%+v", code, checks, stdout.String(), stderr.String(), fake.Calls)
			}
		})
	}
}
