package azcli_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/redact"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	mfaMinimumVersionJSON = `{"azure-cli":"2.61.0"}`
	mfaSubscriptionFlag   = "--subscription"
)

func TestLoginTenantDoesNotInheritAnotherTenantsSubscription(t *testing.T) {
	const id = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, selector := range [][]string{
		{mfaSubscriptionFlag, "11111111-1111-1111-1111-111111111111"},
		{"--subscription=11111111-1111-1111-1111-111111111111"},
	} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
		cli, err := azcli.NewForLogin(fstest.MapFS{}, fake, "")
		if err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		cli.Terminal = &azcli.Terminal{Stdout: &stdout, Stderr: &stderr}
		extra := append([]string{"--tenant=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "--use-device-code"}, selector...)
		extra = append(extra, "--scope", "https://management.azure.com/.default", "--allow-no-subscriptions")
		original := append([]string(nil), extra...)
		result, err := cli.LoginTenant(id, extra)
		want := []string{"login", "--tenant", id, "--use-device-code", "--scope", "https://management.azure.com/.default", "--allow-no-subscriptions"}
		if err != nil || !result.Attempted || len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, want) ||
			!reflect.DeepEqual(extra, original) {
			t.Fatalf("result=%+v error=%v calls=%+v input=%q", result, err, fake.Calls, extra)
		}
	}
}

func TestParseTenantFailuresUsesGUIDWhenDisplayNameIsMissing(t *testing.T) {
	const id = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, label := range []string{"", " ''"} {
		line := "Authentication failed against tenant " + id + label + ": AADSTS50076: MFA required"
		failures := azcli.ParseTenantFailures(line)
		if len(failures) != 1 {
			t.Fatalf("failures=%+v for %q", failures, line)
		}
		failure := failures[0]
		if failure.ID != id || failure.Name != id || failure.Code != "50076" || !failure.Retryable() || failure.Message != line {
			t.Fatalf("failure=%+v", failure)
		}
	}
}

func TestLoginInjectedRunnerReturnsOnlyRedactedTenantDiagnostics(t *testing.T) {
	const secret = "synthetic-sensitive-name"
	stderr := "ERROR: Authentication failed against tenant bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" +
		" 'password=" + secret + "': AADSTS50079: token=" + secret + "\n"
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: mfaMinimumVersionJSON}},
		{Result: azcli.Result{Stderr: stderr, ExitCode: 1}, Err: errors.New("synthetic exit")},
	}}
	cli, err := azcli.NewForLogin(fstest.MapFS{}, fake, "")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cli.Terminal = &azcli.Terminal{Stderr: &output}
	result, err := cli.Login(nil)
	if err == nil || len(result.Failures) != 1 || result.Failures[0].Code != "50079" || len(fake.Calls) != 2 {
		t.Fatalf("result=%+v error=%v calls=%+v", result, err, fake.Calls)
	}
	assertRedactedLoginDiagnostics(t, secret, result.Failures[0].Name, result.Failures[0].Message, output.String(), err.Error())
}

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
			if (err != nil) != failed || !result.Attempted || len(result.Failures) != 4 || attached.String() != redact.Text(stderr) {
				t.Fatalf("result=%+v error=%v attached=%q", result, err, attached.String())
			}
			for i, failure := range result.Failures {
				if failure.Code != wantCodes[i] || failure.Name != names[i] || failure.Retryable() != (i < 2) || failure.Message != redact.Text(lines[i]) {
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
