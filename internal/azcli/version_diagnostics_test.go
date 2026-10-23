package azcli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestMinimumVersionFailureDiagnostics(t *testing.T) {
	failure := errors.New("process failed")
	for _, tc := range []struct {
		stderr, want string
	}{
		{"", "check Azure CLI version: process failed"},
		{" \n\t", "check Azure CLI version: process failed"},
		{" synthetic failure \n", "check Azure CLI version: synthetic failure: process failed"},
	} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stderr: tc.stderr}, Err: failure}}}
		version, err := azcli.CheckVersion(context.Background(), fake)
		if !errors.Is(err, failure) || err.Error() != tc.want || version != "" {
			t.Errorf("stderr=%q: version=%q err=%v want=%q", tc.stderr, version, err, tc.want)
		}
	}
}
