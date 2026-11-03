package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	loginSubscriptionJSON = "[{\"id\":\"synthetic\"}]\n"
	loginFlowTenant       = "targeted tenant"
)

func TestLoginFakeAZTenantFlowsKeepSelectorsAndQuietDefaults(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, flow := range []string{loginFlowTenant, allTenantsFlag, "MFA"} {
		for _, outputSelected := range []bool{false, true} {
			t.Run(flow+map[bool]string{true: "/query", false: "/quiet"}[outputSelected], func(t *testing.T) {
				deps := commandDependencies(t, nil)
				args := []string{loginCommand, deviceCodeFlag}
				initial := []string{loginCommand, deviceCodeFlag}
				query := []string{}
				if outputSelected {
					query = []string{"--query", idFormat}
				}
				if flow == loginFlowTenant {
					args = append(args, tenantFlag, tenantID)
					initial = append(initial, tenantFlag, tenantID)
				} else if flow == allTenantsFlag {
					args = append(args, allTenantsFlag)
				}
				args = append(append(args, argumentDelimiter), query...)
				initial = append(initial, query...)
				const childOutput = "[\"synthetic\"]\n"
				fixtures := []map[string]any{
					{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
					{fixtureArgs: initial, fixtureStdout: childOutput, fixtureStderr: "initial live device code\n"},
				}
				wantStdout := childOutput
				if flow != loginFlowTenant {
					if flow == allTenantsFlag {
						fixtures = append(fixtures, map[string]any{
							fixtureArgs:   []string{accountCommand, tenantSubcommand, listCommand, outputFlag, jsonFormat},
							fixtureStdout: `[{"tenantId":"` + mfaTenant + `"}]`,
						})
					} else {
						fixtures[1][fixtureStderr] = "initial live device code\n" + mfaFixture(t, "50076")
					}
					retry := append([]string{loginCommand, tenantFlag, mfaTenant, deviceCodeFlag}, query...)
					fixtures = append(fixtures, map[string]any{fixtureArgs: retry, fixtureStdout: childOutput, fixtureStderr: "retry live device code\n"})
					wantStdout += childOutput
				}
				if !outputSelected {
					wantStdout = ""
				}
				deps.Runner, _ = configureMFAProcess(t, dir, fixtures)
				var stdout, stderr bytes.Buffer
				code := cmd.Run(args, deps, attachedMFAInput(t), &stdout, &stderr)
				if code != 0 || stdout.String() != wantStdout ||
					!strings.Contains(stderr.String(), "initial live device code\n") ||
					flow != loginFlowTenant && !strings.Contains(stderr.String(), "retry live device code\n") ||
					!strings.Contains(stderr.String(), "✓ Signed in: 1 tenants, 2 subscriptions\n") {
					t.Fatalf("code=%d stdout=%q stderr=%q", code, &stdout, &stderr)
				}
			})
		}
	}
}

func TestLoginFakeAZQuietStdoutAndLiveStderr(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, selectors := range [][]string{
		nil,
		{"-o", jsonFormat},
		{"-ojson"},
		{"-o=json"},
		{"--output", jsonFormat},
		{"--output=" + jsonFormat},
		{"--query", idFormat},
		{"--query=id"},
	} {
		t.Run(strings.Join(selectors, " "), func(t *testing.T) {
			deps := commandDependencies(t, nil)
			args := append([]string{loginCommand, deviceCodeFlag, outputFlag, textFormat, argumentDelimiter}, selectors...)
			wantArgs := append([]string{loginCommand, deviceCodeFlag}, selectors...)
			deps.Runner, _ = configureMFAProcess(t, dir, []map[string]any{
				{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
				{
					fixtureArgs: wantArgs, fixtureStdout: loginSubscriptionJSON,
					fixtureStderr: "Use the device code SYNTHETIC at https://microsoft.com/devicelogin\n",
				},
			})
			var stdout, stderr bytes.Buffer
			code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
			wantStdout := ""
			if len(selectors) != 0 {
				wantStdout = loginSubscriptionJSON
			}
			if code != 0 || stdout.String() != wantStdout ||
				!strings.Contains(stderr.String(), "Use the device code SYNTHETIC") ||
				!strings.Contains(stderr.String(), "✓ Signed in: 1 tenants, 2 subscriptions\n") ||
				!strings.Contains(stderr.String(), "Active subscription: "+tenantID+"/Production ("+tenantID+"/"+productionID+")\n") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, &stdout, &stderr)
			}
		})
	}
}
