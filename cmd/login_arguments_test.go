package cmd_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestLoginArgumentBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		reject bool
		want   []string
	}{
		{name: "positional", args: []string{"unexpected"}, reject: true},
		{name: "positional before delimiter", args: []string{"unexpected", argumentDelimiter, deviceCodeFlag}, reject: true},
		{name: "unknown flag", args: []string{"--not-a-real-flag"}, reject: true},
		{name: "plain", want: []string{loginCommand}},
		{name: "delimited flag", args: []string{argumentDelimiter, deviceCodeFlag}, want: []string{loginCommand, deviceCodeFlag}},
		{name: "delimited tenant name", args: []string{argumentDelimiter, tenantFlag, metadataTenantDomain}, want: []string{loginCommand, tenantFlag, metadataTenantDomain}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: brokerVersionJSON}}, {},
			}}
			deps := commandDependencies(t, fake)
			var stdout, stderr bytes.Buffer
			code := cmd.Run(append([]string{loginCommand}, tc.args...), deps, strings.NewReader(""), &stdout, &stderr)
			if tc.reject {
				if code != cmd.ExitFailure || len(fake.Calls) != 0 || stdout.Len() != 0 || stderr.Len() == 0 {
					t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
				}
				return
			}
			if code != cmd.ExitSuccess || len(fake.Calls) != 2 ||
				!reflect.DeepEqual(fake.Calls[0].Args, []string{versionCommand, outputFlag, jsonFormat}) ||
				!reflect.DeepEqual(fake.Calls[1].Args, tc.want) || !fake.Calls[1].Attached {
				t.Fatalf("code=%d stderr=%q calls=%+v", code, stderr.String(), fake.Calls)
			}
		})
	}
}
