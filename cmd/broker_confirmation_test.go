package cmd_test

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBrokerCannotConfirmInvisiblePrompt(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {}, {}, {}}}
	deps := commandDependencies(t, fake)
	deps.Platform, deps.FS = platformWindows, fstest.MapFS{}
	var stdout bytes.Buffer
	code := cmd.Run([]string{authCommand, brokerCommand, brokerEnable}, deps, strings.NewReader("yes\n"), &stdout, doctorFailedWriter{})
	if code != 1 || len(fake.Calls) != 1 || stdout.Len() != 0 {
		t.Fatalf("invisible confirmation triggered changes: code=%d calls=%+v stdout=%q", code, fake.Calls, stdout.String())
	}
}

func TestBrokerConfirmationInterruptAndOversizedInput(t *testing.T) {
	for _, input := range []string{"\x03", strings.Repeat("y", 257) + "\n"} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}}}
		deps := commandDependencies(t, fake)
		deps.Platform, deps.FS = platformWindows, fstest.MapFS{}
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{authCommand, brokerCommand, brokerEnable}, deps, strings.NewReader(input), &stdout, &stderr)
		want := 1
		if input == "\x03" {
			want = 130
		}
		if code != want || len(fake.Calls) != 1 || stdout.Len() != 0 {
			t.Fatalf("confirmation code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
		}
	}
}
