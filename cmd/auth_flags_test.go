package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestFriendlyTenantRejectsConflictingPassthrough(t *testing.T) {
	for _, extra := range [][]string{{tenantFlag, tenantID}, {"-t", tenantID}, {"--tenant=" + tenantID}, {"-t" + tenantID}} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		args := []string{loginCommand, tenantFlag, tenantID}
		args = append(args, argumentDelimiter)
		args = append(args, extra...)
		var stdout, stderr bytes.Buffer
		code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
		if code != 1 || len(fake.Calls) != 0 || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q calls=%+v", args, code, stdout.String(), stderr.String(), fake.Calls)
		}
	}
}

func TestFirstLoginWithFriendlyFlags(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {}}}
	deps := commandDependencies(t, fake)
	deps.ConfigDir, deps.FS = filepath.Join(testutil.WorkDir(t), "azure"), nil
	deps.Runner = firstLoginRunner{fake: fake, configDir: deps.ConfigDir}
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand, deviceCodeFlag, tenantFlag, tenantID}, deps, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || len(fake.Calls) != 2 || !strings.Contains(stderr.String(), "device code") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
	assertFirstLoginFiles(t, deps.ConfigDir, false, true)
}
