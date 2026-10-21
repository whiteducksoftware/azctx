package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAzureCLIVersionFailuresBlockOnlyLogin(t *testing.T) {
	for _, response := range []testutil.Response{
		{Result: azcli.Result{Stdout: `{"azure-cli":"2.60.0"}`}},
		{Result: azcli.Result{Stdout: `{"azure-cli":"malformed"}`}},
		{Result: azcli.Result{Stdout: `{`}},
		{Result: azcli.Result{Stderr: "version unavailable", ExitCode: 1}, Err: errors.New("exit status 1")},
	} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{response}}
		deps := commandDependencies(t, fake)
		var stdout, stderr bytes.Buffer
		if code := cmd.Run([]string{loginCommand}, deps, bytes.NewReader(nil), &stdout, &stderr); code != 1 || stdout.Len() != 0 {
			t.Fatalf("version failure: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if len(fake.Calls) != 1 || !reflect.DeepEqual(fake.Calls[0].Args, []string{versionCommand, outputFlag, jsonFormat}) {
			t.Fatalf("login passed version failure: %+v", fake.Calls)
		}
	}
}

func TestAzureCLISharedJSONProjection(t *testing.T) {
	deps := commandDependencies(t, &testutil.FakeRunner{})
	deps.StdinTTY = func() bool { return false }
	deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(strings.ReplaceAll(modernProfile, `"name":"Delegated"`, `"name":"Production"`))}}
	run := func(args []string, wantCode int) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr)
		if code != wantCode {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
		if wantCode == 0 {
			if stderr.Len() != 0 {
				t.Fatalf("data diagnostics=%q", stderr.String())
			}
			return stdout.Bytes()
		}
		if stdout.Len() != 0 {
			t.Fatalf("error data=%q", stdout.String())
		}
		return stderr.Bytes()
	}
	var list struct {
		Subscriptions []cmd.SubscriptionOutput `json:"subscriptions"`
	}
	var current struct {
		Subscription cmd.SubscriptionOutput `json:"subscription"`
	}
	var failure struct {
		Candidates []cmd.SubscriptionOutput `json:"candidates"`
	}
	for _, item := range []struct {
		args []string
		code int
		dst  any
	}{
		{[]string{listCommand, outputFlag, jsonFormat}, 0, &list},
		{[]string{currentFlag, outputFlag, jsonFormat}, 0, &current},
		{[]string{productionName, verboseFlag, outputFlag, jsonFormat}, 2, &failure},
	} {
		if err := json.Unmarshal(run(item.args, item.code), item.dst); err != nil {
			t.Fatal(err)
		}
	}
	if len(list.Subscriptions) != 4 || len(failure.Candidates) != 2 ||
		!reflect.DeepEqual(current.Subscription, list.Subscriptions[0]) ||
		!reflect.DeepEqual(failure.Candidates, list.Subscriptions[:2]) {
		t.Fatalf("inconsistent projection: list=%+v current=%+v candidates=%+v", list, current, failure)
	}
}

func TestAzureCLIPreviousAndSelectionRespectCloud(t *testing.T) {
	for _, cloud := range []string{azureCloudName, customCloudName} {
		for _, query := range []string{productionID, "44444444-4444-4444-4444-444444444444", "-"} {
			t.Run(cloud+"/"+query, func(t *testing.T) {
				fake := &testutil.FakeRunner{Responses: []testutil.Response{{}}}
				deps := commandDependencies(t, fake)
				t.Setenv("AZURE_CLOUD_NAME", cloud)
				deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(modernProfile)}}
				if err := state.Save(deps.ConfigDir, productionID); err != nil {
					t.Fatal(err)
				}
				wantSuccess := (query == productionID || query == "-") == (cloud == azureCloudName)
				var stdout, stderr bytes.Buffer
				code := cmd.Run([]string{query}, deps, bytes.NewReader(nil), &stdout, &stderr)
				wantCalls := 0
				if wantSuccess {
					wantCalls = 1
				}
				if (code == 0) != wantSuccess || len(fake.Calls) != wantCalls {
					t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
				}
				if wantSuccess {
					currentID := productionID
					if cloud == customCloudName {
						currentID = "44444444-4444-4444-4444-444444444444"
					}
					if got := fake.Calls[0].Args; !reflect.DeepEqual(got, []string{accountCommand, setCommand, subscriptionFlag, currentID}) {
						t.Fatalf("switch argv=%q", got)
					}
				}
			})
		}
	}
}
