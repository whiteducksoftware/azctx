package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

// Model Azure CLI's profile changes at the runner boundary, only in memory.
type defaultChangingRunner struct {
	azcli.Runner
	files  fstest.MapFS
	logins int
}

func (r *defaultChangingRunner) Run(ctx context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	result, err := r.Runner.Run(ctx, args, options)
	if args[0] == loginCommand {
		r.logins++
		id := developmentID
		if r.logins == 1 {
			id = productionID
		}
		r.files[profileFile] = &fstest.MapFile{Data: []byte(loginDefaultProfile(id))}
	} else if len(args) == 4 && args[0] == accountCommand && args[1] == setCommand && err == nil {
		r.files[profileFile] = &fstest.MapFile{Data: []byte(loginDefaultProfile(args[3]))}
	}
	return result, err
}

func loginDefaultProfile(id string) string {
	return `{"subscriptions":[
		{"id":"` + productionID + `","tenantId":"` + tenantID + `","environmentName":"AzureCloud","isDefault":` +
		strconv.FormatBool(id == productionID) + `},
		{"id":"` + developmentID + `","tenantId":"` + mfaTenant + `","environmentName":"AzureCloud","isDefault":` +
		strconv.FormatBool(id == developmentID) + `}]}`
}

func TestMFADefaultRestorationKeepsSignInFailures(t *testing.T) {
	for _, retryFailed := range []bool{true, false} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{
			{Result: azcli.Result{Stdout: brokerVersionJSON}},
			{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
			{},
			{},
		}}
		failureIndex := 3
		if retryFailed {
			failureIndex = 2
		}
		fake.Responses[failureIndex].Err = errors.New("synthetic process failure")
		deps := commandDependencies(t, fake)
		files := fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(loginDefaultProfile(developmentID))}}
		deps.FS, deps.Runner = files, &defaultChangingRunner{Runner: fake, files: files}
		var stdout, stderr bytes.Buffer
		code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("yes\n"), &stdout, &stderr)
		if code != 3 || len(fake.Calls) != 4 || strings.Contains(stderr.String(), "Successfully fetched") ||
			!strings.Contains(strings.Join(fake.Calls[3].Args, " "), "account set --subscription "+productionID) {
			t.Fatalf("code=%d calls=%+v stderr=%q", code, fake.Calls, stderr.String())
		}
		if retryFailed && string(files[profileFile].Data) != loginDefaultProfile(productionID) {
			t.Fatalf("partial failure lost initial default: %s", files[profileFile].Data)
		}
	}
}

func TestMFADefaultRestorationRejectsNonGUIDProfileID(t *testing.T) {
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: brokerVersionJSON}},
		{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
		{},
	}}
	deps := commandDependencies(t, fake)
	files := fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(strings.ReplaceAll(fixtureProfile, productionID, "not-a-guid"))}}
	deps.FS, deps.Runner = files, refreshedLoginRunner{fake: fake, files: files}
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{loginCommand}, deps, strings.NewReader("yes\n"), &stdout, &stderr)
	if code != 3 || len(fake.Calls) != 3 || !strings.Contains(stderr.String(), "could not restore the initial default subscription") {
		t.Fatalf("code=%d calls=%+v stderr=%q", code, fake.Calls, stderr.String())
	}
}

func TestMFATenantLoginRestoresInitialLoginDefault(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, all := range []bool{false, true} {
		for _, selector := range [][]string{
			{subscriptionFlag, productionID},
			{"--subscription=" + productionID},
			nil,
		} {
			t.Run(strings.Join(selector, " ")+map[bool]string{true: "/all", false: "/retry"}[all], func(t *testing.T) {
				deps := commandDependencies(t, &testutil.FakeRunner{})
				initial := append([]string{loginCommand, deviceCodeFlag}, selector...)
				fixtures := []map[string]any{
					{fixtureArgs: []string{versionCommand, outputFlag, jsonFormat}, fixtureStdout: brokerVersionJSON},
					{fixtureArgs: initial},
				}

				args := []string{loginCommand, deviceCodeFlag}
				if all {
					args = append(args, allTenantsFlag)
					fixtures = append(fixtures, map[string]any{
						fixtureArgs:   []string{accountCommand, tenantSubcommand, listCommand, outputFlag, jsonFormat},
						fixtureStdout: `[{"tenantId":"` + mfaTenant + `"}]`,
					})
				} else {
					fixtures[1][fixtureStderr] = strings.ReplaceAll(mfaFixture(t, "50076"), "MFA Directory", "Password")
				}
				fixtures = append(fixtures,
					map[string]any{fixtureArgs: []string{loginCommand, tenantFlag, mfaTenant, deviceCodeFlag}},
					map[string]any{fixtureArgs: []string{accountCommand, setCommand, subscriptionFlag, productionID}},
				)
				runner, record := configureMFAProcess(t, dir, fixtures)
				profile := fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(loginDefaultProfile(developmentID))}}
				deps.FS, deps.Runner = profile, &defaultChangingRunner{Runner: runner, files: profile}
				if len(selector) != 0 {
					args = append(append(args, argumentDelimiter), selector...)
				}
				var stdout, stderr bytes.Buffer
				code := cmd.Run(args, deps, attachedMFAInput(t), &stdout, &stderr)
				calls, err := os.ReadFile(record)
				if code != 0 || err != nil || strings.Count(string(calls), "\n") != len(fixtures) ||
					string(profile[profileFile].Data) != loginDefaultProfile(productionID) {
					t.Fatalf("code=%d error=%v calls=%s profile=%s stderr=%q", code, err, calls, profile[profileFile].Data, stderr.String())
				}
			})
		}
	}
}
