package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	mfaTenant          = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	registrationTenant = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	blockedTenant      = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	allTenantsFlag     = "--all-tenants"
	tenantSubcommand   = "tenant"
	otherMFAFixture    = "other"
)

func mfaFixture(t *testing.T, code string) string {
	t.Helper()
	data, err := os.ReadFile("../internal/azcli/testdata/mfa/" + code + ".stderr.golden")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMFALoginPartialFailuresUseExitThree(t *testing.T) {
	for _, tc := range []struct {
		name, fixture, env, input string
		args                      []string
		noTTY, missingProfile     bool
		processError              bool
	}{
		{name: "non-tty", fixture: "50076", noTTY: true},
		{name: "no-input", fixture: "50076", args: []string{noInputFlag}},
		{name: "environment", fixture: "50079", env: "AZCTX_NO_INPUT"},
		{name: "ci", fixture: "50076", env: "CI"},
		{name: "blocked", fixture: "53003"},
		{name: "unclassified", fixture: otherMFAFixture},
		{name: "user declined", fixture: "50076", input: "n\n"},
		{name: "failed process without warning", processError: true},
		{name: "failed process without profile", fixture: "50076", processError: true, missingProfile: true, noTTY: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := testutil.Response{}
			if tc.fixture != "" {
				response.Result.Stderr = mfaFixture(t, tc.fixture)
			}
			if tc.processError {
				response.Err = errors.New("synthetic sign-in failure")
				response.Result.ExitCode = 1
			}
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: brokerVersionJSON}}, response,
			}}
			deps := commandDependencies(t, fake)
			if tc.noTTY {
				deps.StdinTTY = func() bool { return false }
			}
			if tc.env != "" {
				t.Setenv(tc.env, "1")
			}
			if tc.missingProfile {
				deps.FS = fstest.MapFS{}
			}
			var stdout, stderr bytes.Buffer
			code := cmd.Run(append([]string{loginCommand}, tc.args...), deps, strings.NewReader(tc.input), &stdout, &stderr)
			if code != 3 || len(fake.Calls) != 2 || stdout.Len() != 0 || strings.Contains(stderr.String(), "✓ Signed in:") {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
			if tc.name != "user declined" && strings.Contains(stderr.String(), "[Y/n]") {
				t.Fatalf("unexpected confirmation: %q", stderr.String())
			}
			assertMFAFailureDetails(t, tc.fixture, response.Result.Stderr, stderr.String())
		})
	}
}

func assertMFAFailureDetails(t *testing.T, fixture, raw, stderr string) {
	t.Helper()
	if fixture != "" && !strings.Contains(stderr, "TENANT ID") {
		t.Fatalf("missing failure table: %q", stderr)
	}
	if fixture == "53003" && !strings.Contains(stderr, "retry will not help") {
		t.Fatalf("missing Conditional Access explanation: %q", stderr)
	}
	if fixture == otherMFAFixture && strings.Count(stderr, strings.TrimSpace(raw)) != 2 {
		t.Fatalf("unknown failure lost verbatim table diagnostic: %q", stderr)
	}
}

func TestMFALoginRetriesOnlyFailedTenantsWithOneConfirmation(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076") + mfaFixture(t, "50079")}},
		{},
		{},
	}}
	deps := commandDependencies(t, fake)
	files, ok := deps.FS.(fstest.MapFS)
	if !ok {
		t.Fatal("fixture is not a MapFS")
	}
	files[profileFile].Data = []byte(`{"subscriptions":[
		{"id":"11111111-1111-1111-1111-111111111111","tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","environmentName":"AzureCloud"},
		{"id":"22222222-2222-2222-2222-222222222222","tenantId":"` + mfaTenant + `","environmentName":"AzureCloud"},
		{"id":"33333333-3333-3333-3333-333333333333","tenantId":"` + registrationTenant + `","environmentName":"AzureCloud"}]}`)
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand, deviceCodeFlag}, deps, strings.NewReader("\n"), &stdout, &stderr)
	want := [][]string{
		{versionCommand, outputFlag, jsonFormat},
		{loginCommand, deviceCodeFlag},
		{loginCommand, tenantFlag, mfaTenant, deviceCodeFlag},
		{loginCommand, tenantFlag, registrationTenant, deviceCodeFlag},
	}
	var got [][]string
	for _, call := range fake.Calls {
		got = append(got, call.Args)
	}
	if code != 0 || !reflect.DeepEqual(got, want) || strings.Count(stderr.String(), "Sign in to these 2 tenants now? [Y/n]") != 1 ||
		!strings.Contains(stderr.String(), "✓ Signed in: 3 tenants, 3 subscriptions\n") || stdout.Len() != 0 {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
	for _, call := range fake.Calls[1:] {
		if !call.Attached || !reflect.DeepEqual(call.Env, []string{childPickerOff, childConfigDirPrefix + deps.ConfigDir}) {
			t.Fatalf("detached login or incorrect environment: %+v", call)
		}
	}
}

func TestAllTenantsDiscoversInsteadOfUsingProfile(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{},
		{Result: azcli.Result{Stdout: `[{"tenantId":"` + mfaTenant + `"},{"tenantId":"` + strings.ToUpper(mfaTenant) + `"},{"tenantId":"` + registrationTenant + `"}]`}},
		{},
		{},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand, allTenantsFlag, deviceCodeFlag}, deps, strings.NewReader(""), &stdout, &stderr)
	want := [][]string{
		{versionCommand, outputFlag, jsonFormat},
		{loginCommand, deviceCodeFlag},
		{accountCommand, tenantSubcommand, listCommand, outputFlag, jsonFormat},
		{loginCommand, tenantFlag, mfaTenant, deviceCodeFlag},
		{loginCommand, tenantFlag, registrationTenant, deviceCodeFlag},
	}
	var got [][]string
	for _, call := range fake.Calls {
		got = append(got, call.Args)
	}
	if code != 0 || !reflect.DeepEqual(got, want) || strings.Contains(stderr.String(), "[Y/n]") {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
	if fake.Calls[2].Attached || !reflect.DeepEqual(fake.Calls[2].Env, []string{
		"AZURE_EXTENSION_USE_DYNAMIC_INSTALL=no", childConfigDirPrefix + deps.ConfigDir,
	}) {
		t.Fatalf("tenant discovery must remain captured with isolated settings: %+v", fake.Calls[2])
	}
}

func TestWorkloadLoginSkipsMFARetriesAndDiscovery(t *testing.T) {
	for _, identity := range []string{"--identity", "-i", "--identity=true", "--service-principal", "--service-principal=true"} {
		for _, failed := range []bool{false, true} {
			t.Run(identity+map[bool]string{true: "/failed", false: "/success"}[failed], func(t *testing.T) {
				response := testutil.Response{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}}
				const secret = "synthetic-password-never-print"
				if failed {
					response.Err = errors.New("process failed with private argument " + secret)
				}
				fake := &testutil.FakeRunner{Responses: []testutil.Response{
					{Result: azcli.Result{Stdout: brokerVersionJSON}}, response,
				}}
				deps := commandDependencies(t, fake)
				args := []string{loginCommand, allTenantsFlag, argumentDelimiter, identity, "--password", secret}
				var stdout, stderr bytes.Buffer
				code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
				want := 0
				if failed {
					want = 3
				}
				expected := append([]string{loginCommand}, args[3:]...)
				if code != want || len(fake.Calls) != 2 || !reflect.DeepEqual(fake.Calls[1].Args, expected) ||
					strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), "[Y/n]") || strings.Contains(stderr.String(), "TENANT ID") {
					t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestTenantLoginTracksNewFailuresAndContinuesAfterPartialFailure(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{true: "all", false: "targeted retry"}[all], func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{{Result: azcli.Result{Stdout: brokerVersionJSON}}}}
			args := []string{loginCommand}
			if all {
				args = append(args, allTenantsFlag)
				fake.Responses = append(fake.Responses, testutil.Response{}, testutil.Response{Result: azcli.Result{
					Stdout: `[{"tenantId":"` + mfaTenant + `"},{"tenantId":"` + registrationTenant + `"}]`,
				}})
			} else {
				fake.Responses = append(fake.Responses, testutil.Response{Result: azcli.Result{
					Stderr: mfaFixture(t, "50076") + mfaFixture(t, "50079"),
				}})
			}
			fake.Responses = append(fake.Responses,
				testutil.Response{Err: errors.New("synthetic child failure")},
				testutil.Response{},
			)
			deps := commandDependencies(t, fake)
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader("yes\n"), &stdout, &stderr)
			wantCalls := 4
			if all {
				wantCalls++
			}
			if code != 3 || len(fake.Calls) != wantCalls || !strings.Contains(stderr.String(), "sign-in failed for 1 tenants") ||
				!strings.Contains(stderr.String(), "Fetched 1 tenants and 2 subscriptions; sign-in incomplete.") {
				t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
			}
		})
	}
}

func TestTenantRetryUpdatesConditionalAccessClassification(t *testing.T) {
	updated := strings.ReplaceAll(mfaFixture(t, "53003"), blockedTenant, mfaTenant)
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
		{Result: azcli.Result{Stderr: updated}, Err: errors.New("policy failure")},
	}}
	deps := commandDependencies(t, fake)
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("Y\n"), &stdout, &stderr)
	if code != 3 || len(fake.Calls) != 3 || !strings.Contains(stderr.String(), "retry will not help") ||
		strings.Count(stderr.String(), "[Y/n]") != 1 {
		t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
	}
}
