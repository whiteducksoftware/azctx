package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAllTenantsRejectsInvalidDiscoveryBeforeTenantLogin(t *testing.T) {
	for _, data := range []string{"invalid", "null", `{}`, `[{"id":"` + mfaTenant + `"}]`, `[{"tenantId":"` + mfaTenant + `"},{"tenantId":"--not-a-guid"}]`} {
		t.Run(data, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
				{},
				{Result: azcli.Result{Stdout: data}},
			}}
			deps := commandDependencies(t, fake)
			var stdout, stderr bytes.Buffer
			if code := cmd.Run([]string{loginCommand, allTenantsFlag}, deps, strings.NewReader(""), &stdout, &stderr); code != 1 ||
				len(fake.Calls) != 3 || !strings.Contains(stderr.String(), "az account tenant list returned an invalid tenant") {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}

func TestAllTenantsRejectsCompetingTenantSelectors(t *testing.T) {
	for _, selector := range [][]string{
		{tenantFlag, tenantID},
		{argumentDelimiter, tenantFlag, tenantID},
		{argumentDelimiter, "-t" + tenantID},
	} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		var stdout, stderr bytes.Buffer
		if code := cmd.Run(append([]string{loginCommand, allTenantsFlag}, selector...), deps, strings.NewReader(""), &stdout, &stderr); code != 1 ||
			len(fake.Calls) != 0 || !strings.Contains(stderr.String(), "cannot be combined with a tenant selector") {
			t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
		}
	}
}

func TestTenantRetrySkipsNewlyBlockedTenant(t *testing.T) {
	blocked := strings.ReplaceAll(mfaFixture(t, "53003"), blockedTenant, registrationTenant)
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076") + mfaFixture(t, "50079")}},
		{Result: azcli.Result{Stderr: blocked}},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("yes\n"), &stdout, &stderr); code != 3 ||
		len(fake.Calls) != 3 || !strings.Contains(stderr.String(), "sign-in failed for 1 tenants") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}

func TestMFAConfirmationNeverRetriesAfterReadFailureOrInterrupt(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		code        int
	}{
		{name: "EOF", code: 3},
		{name: "too long", input: strings.Repeat("y", 256), code: 3},
		{name: "Ctrl+C", input: "\x03", code: 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
				{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
			}}
			deps := commandDependencies(t, fake)
			var stdout, stderr bytes.Buffer
			if code := cmd.Run([]string{loginCommand}, deps, strings.NewReader(tc.input), &stdout, &stderr); code != tc.code || len(fake.Calls) != 2 {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRemovedMFAFlagKeepsSingleJSONErrorBoundary(t *testing.T) {
	for _, flag := range []string{"--force-mfa", "--force-mfa=false"} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{loginCommand, flag, outputFlag, jsonFormat}, deps, strings.NewReader(""), &stdout, &stderr)
		want := "{\"code\":\"error\",\"message\":\"unknown flag: --force-mfa\",\"exit_code\":1}\n"
		if code != 1 || stderr.String() != want || stdout.Len() != 0 || len(fake.Calls) != 0 {
			t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
		}
	}
}

func TestSignInFailureCauseKeepsCentralJSONClassification(t *testing.T) {
	cause := errors.New("synthetic process failure")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}}, {Err: cause},
	}}
	command := cmd.NewRootCommand(commandDependencies(t, fake))
	command.SetArgs([]string{loginCommand})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	err := command.Execute()
	if !errors.Is(err, cause) {
		t.Fatalf("lost process cause: %v", err)
	}
	output.Reset()
	if code := cmd.WriteError(&output, jsonFormat, err); code != 3 || strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("code=%d output=%q", code, output.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil || payload.Code != cmd.CodeSignInFailed {
		t.Fatalf("invalid error classification: payload=%+v error=%v", payload, err)
	}
}

type refreshedLoginRunner struct {
	fake  *testutil.FakeRunner
	files fstest.MapFS
}

func (runner refreshedLoginRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	result, err := runner.fake.Run(ctx, args, options)
	if len(runner.fake.Calls) == 3 {
		runner.files[profileFile].Data = []byte(`{"subscriptions":[
			{"id":"11111111-1111-1111-1111-111111111111","tenantId":"` + tenantID + `","environmentName":"AzureCloud"},
			{"id":"22222222-2222-2222-2222-222222222222","tenantId":"` + mfaTenant + `","environmentName":"AzureCloud"}]}`)
	}
	return result, err
}

func TestMFASummaryStrictlyReloadsAfterLastLogin(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
		{},
		{},
	}}
	deps := commandDependencies(t, fake)
	files, ok := deps.FS.(fstest.MapFS)
	if !ok {
		t.Fatal("fixture is not a MapFS")
	}
	deps.Runner = refreshedLoginRunner{fake: fake, files: files}
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("yes\n"), &stdout, &stderr); code != 0 ||
		!strings.Contains(stderr.String(), "Successfully fetched 2 tenants and 2 subscriptions.") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}
