package cmd_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type doctorFailedWriter struct{}

func (doctorFailedWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }

func TestDoctorTextAndOutputFailure(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}, {Result: azcli.Result{Stdout: brokerVersionJSON}}}}
	deps := commandDependencies(t, fake)
	deps.Platform, deps.ConfigDir = platformWindows, "synthetic'\x1b[31m\nazure"
	deps.FS = fstest.MapFS{
		profileFile:       &fstest.MapFile{Data: []byte(modernProfile)},
		retiredTenantFile: &fstest.MapFile{Data: []byte("secret-legacy")},
	}
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{doctorCommand}, deps, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "Remove-Item -LiteralPath") ||
		strings.ContainsAny(stdout.String(), "\x1b") || strings.Contains(stdout.String(), "synthetic@example.invalid") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	code = cmd.Run([]string{doctorCommand}, deps, strings.NewReader(""), doctorFailedWriter{}, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "output failed") {
		t.Fatalf("write failure code=%d stderr=%q", code, stderr.String())
	}
}
