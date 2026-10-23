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

func TestAllTenantsNoInputStopsAfterInitialTenantFailures(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand, allTenantsFlag, noInputFlag}, deps, strings.NewReader("y\n"), &stdout, &stderr); code != 3 ||
		len(fake.Calls) != 2 || strings.Contains(stderr.String(), "[Y/n]") || !strings.Contains(stderr.String(), mfaTenant) {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}

func TestMFAWarningsDeduplicateTenantIdentityButRetainDiagnostics(t *testing.T) {
	warnings := mfaFixture(t, "50076") + strings.ReplaceAll(
		strings.ReplaceAll(mfaFixture(t, "50079"), registrationTenant, strings.ToUpper(mfaTenant)),
		"Registration Directory", "Another label")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: warnings}},
		{},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("yes\n"), &stdout, &stderr); code != 0 ||
		len(fake.Calls) != 3 || !strings.Contains(stderr.String(), "Sign in to these 1 tenants now? [Y/n]") ||
		!strings.Contains(stderr.String(), "MFA registration required") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}

func TestAllTenantsDoesNotRepeatKnownConditionalAccessBlocks(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "53003")}},
		{Result: azcli.Result{Stdout: `[{"tenantId":"` + blockedTenant + `"},{"tenantId":"` + mfaTenant + `"}]`}},
		{},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand, allTenantsFlag}, deps, strings.NewReader(""), &stdout, &stderr); code != 3 ||
		len(fake.Calls) != 4 || !reflect.DeepEqual(fake.Calls[3].Args, []string{loginCommand, tenantFlag, mfaTenant}) ||
		strings.Contains(stderr.String(), "[Y/n]") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}

func TestWorkloadLoginStillRejectsUnsupportedAzureVersion(t *testing.T) {
	const unsupportedVersionJSON = `{"azure-cli":"2.60.0"}`
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: unsupportedVersionJSON}}}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{loginCommand, allTenantsFlag, argumentDelimiter, "--identity"}, deps, strings.NewReader(""), &stdout, &stderr); code != 1 ||
		len(fake.Calls) != 1 || !strings.Contains(stderr.String(), "2.61.0 or newer is required") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}
