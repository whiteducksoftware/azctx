package cmd_test

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestContractCommandNameQueriesRequireDelimiter(t *testing.T) {
	for _, name := range []string{listCommand, loginCommand, versionCommand, "help", "completion", verboseFlag} {
		t.Run(name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
			deps := commandDependencies(t, fake)
			deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(strings.ReplaceAll(fixtureProfile, productionName, name))}}
			var stdout, stderr bytes.Buffer
			code := cmd.Run([]string{argumentDelimiter, name}, deps, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 || len(fake.Calls) != 1 || fake.Calls[0].Args[3] != productionID {
				t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
			}
		})
	}
}
