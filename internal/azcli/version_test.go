package azcli_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestMinimumVersion(t *testing.T) {
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{`{"azure-cli":"2.61.0","azure-cli-core":"ignored"}`, true},
		{`{"azure-cli":"2.61.1"}`, true},
		{`{"azure-cli":"2.100.0"}`, true},
		{`{"azure-cli":"3.0.0"}`, true},
		{`{"azure-cli":"2.60.9"}`, false},
		{`{"azure-cli":"1.100.0"}`, false},
		{`{"azure-cli":"2.61"}`, false},
		{`{"azure-cli":"2.61.0b1"}`, false},
		{`{"azure-cli":"latest"}`, false},
		{`{"azure-cli":261}`, false},
		{`{"azure-cli-core":"2.61.0"}`, false},
		{`{}`, false},
		{`null`, false},
		{`{`, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: tc.value}}}}
			version, err := azcli.CheckVersion(context.Background(), fake)
			if (err == nil) != tc.ok || tc.ok && version == "" {
				t.Fatalf("version=%q, error=%v, want success=%v", version, err, tc.ok)
			}
			if len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, []string{"version", "-o", jsonOutput}) ||
				fake.Calls[0].Attached || len(fake.Calls[0].Env) != 0 {
				t.Fatalf("version invocation=%+v", fake.Calls)
			}
		})
	}
	failure := errors.New("process failed")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stderr: "synthetic failure"}, Err: failure}}}
	if _, err := azcli.CheckVersion(context.Background(), fake); !errors.Is(err, failure) || !strings.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("lost command failure: %v", err)
	}
}

func TestLoginChecksVersionAndScopesPickerEnvironment(t *testing.T) {
	t.Setenv("AZURE_CORE_LOGIN_EXPERIENCE_V2", boolOn)
	for _, version := range []string{"2.60.0", "2.61.0"} {
		t.Run(version, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: `{"azure-cli":"` + version + `"}`}}, {},
			}}
			cli, err := azcli.New(fstest.MapFS{profileFilename: &fstest.MapFile{Data: []byte(`{}`)}}, fake, "")
			if err != nil {
				t.Fatal(err)
			}
			err = cli.InteractiveLogin(nil)
			if version == "2.60.0" {
				if err == nil || len(fake.Calls) != 1 {
					t.Fatalf("old CLI login: error=%v, calls=%+v", err, fake.Calls)
				}
			} else if err != nil || len(fake.Calls) != 2 ||
				!reflect.DeepEqual(fake.Calls[1].Env, []string{"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"}) || !fake.Calls[1].Attached {
				t.Fatalf("scoped login: error=%v, calls=%+v", err, fake.Calls)
			}
			if got := os.Getenv("AZURE_CORE_LOGIN_EXPERIENCE_V2"); got != boolOn {
				t.Fatalf("parent environment changed: %q", got)
			}
		})
	}
}
