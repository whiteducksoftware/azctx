package azcli_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const mfaMinimumVersionJSON = `{"azure-cli":"2.61.0"}`

func TestLoginParsesDocumentedTenantFailuresOnBothProcessStatuses(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			codes := []string{"50076", "50079", "53003", "other"}
			names := []string{"MFA Directory", "Registration Directory", "Blocked Directory", "Other Directory"}
			wantCodes := []string{"50076", "50079", "53003", "50105"}
			var lines []string
			for _, code := range codes {
				data, err := os.ReadFile("testdata/mfa/" + code + ".stderr.golden")
				if err != nil {
					t.Fatal(err)
				}
				lines = append(lines, strings.TrimSuffix(string(data), "\n"))
			}
			stderr := strings.Join(lines, "\r\n") + "\r\n"
			response := testutil.Response{Result: azcli.Result{Stderr: stderr}}
			if failed {
				response.Err = errors.New("synthetic process exit")
			}
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: mfaMinimumVersionJSON}}, response,
			}}
			cli, err := azcli.NewForLogin(fstest.MapFS{}, fake, "")
			if err != nil {
				t.Fatal(err)
			}
			var attached bytes.Buffer
			cli.Terminal = &azcli.Terminal{Stdout: &attached, Stderr: &attached}
			result, err := cli.Login(nil)
			if (err != nil) != failed || !result.Attempted || len(result.Failures) != 4 || attached.String() != stderr {
				t.Fatalf("result=%+v error=%v attached=%q", result, err, attached.String())
			}
			for i, failure := range result.Failures {
				if failure.Code != wantCodes[i] || failure.Name != names[i] || failure.Retryable() != (i < 2) || failure.Message != lines[i] {
					t.Fatalf("failure[%d]=%+v", i, failure)
				}
			}
		})
	}
}

func TestLoginIgnoresUnstructuredMFAAndInvalidTenantWarnings(t *testing.T) {
	stderr := "mfa and multi-factor authentication are mentioned here\n" +
		"Authentication failed against tenant ------------------------------------ 'Bad': AADSTS50076: MFA\n" +
		"Authentication failed against tenant --bad-id 'Bad': AADSTS50076: MFA\n" +
		"not a warning: Authentication failed against tenant aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa 'Bad': AADSTS50076: MFA\n"
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: mfaMinimumVersionJSON}},
		{Result: azcli.Result{Stderr: stderr}},
	}}
	cli, err := azcli.NewForLogin(fstest.MapFS{}, fake, "")
	if err != nil {
		t.Fatal(err)
	}
	var attached bytes.Buffer
	cli.Terminal = &azcli.Terminal{Stderr: &attached}
	result, err := cli.Login(nil)
	if err != nil || len(result.Failures) != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}
