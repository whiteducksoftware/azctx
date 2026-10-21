package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAzureCLICloudDefaultRecordsCorrectPrevious(t *testing.T) {
	for _, cloud := range []string{azureCloudName, "", " \t "} {
		t.Run(cloud, func(t *testing.T) {
			testCloudDefaultRecordsPrevious(t, cloud)
		})
	}
}

func testCloudDefaultRecordsPrevious(t *testing.T, cloud string) {
	t.Helper()
	fake := &testutil.FakeRunner{Responses: []testutil.Response{{}, {}}}
	deps := commandDependencies(t, fake)
	t.Setenv("AZURE_CLOUD_NAME", cloud)
	data := strings.Replace(modernProfile, `"isDefault":true`, `"isDefault":false`, 1)
	deps.FS = fstest.MapFS{
		profileFile:      &fstest.MapFile{Data: []byte(data)},
		azureConfigFile:  &fstest.MapFile{Data: []byte("[cloud]\nname= \t")},
		cloudsConfigFile: &fstest.MapFile{Data: []byte("[AzureCloud]\nsubscription=" + productionID)},
	}
	for _, args := range [][]string{{currentFlag, outputFlag, idFormat}, {listCommand, outputFlag, jsonFormat}, {developmentID}} {
		var stdout, stderr bytes.Buffer
		if code := cmd.Run(args, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		switch args[0] {
		case currentFlag:
			if stdout.String() != productionID+"\n" {
				t.Fatalf("cloud current=%q", stdout.String())
			}
		case listCommand:
			var list struct {
				Subscriptions []cmd.SubscriptionOutput `json:"subscriptions"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if len(list.Subscriptions) != 4 || !list.Subscriptions[0].IsDefault {
				t.Fatalf("cloud default list=%s", stdout.String())
			}
		}
	}
	if previous, err := state.Load(deps.ConfigDir); err != nil || previous != productionID {
		t.Fatalf("previous from active cloud=%q, %v", previous, err)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].Args[3] != developmentID {
		t.Fatalf("cloud switch=%+v", fake.Calls)
	}
	assertCloudPreviousSwitch(t, deps, fake)
}

func assertCloudPreviousSwitch(t *testing.T, deps cmd.Dependencies, fake *testutil.FakeRunner) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := cmd.Run([]string{"-"}, deps, bytes.NewReader(nil), &stdout, &stderr); code != 0 ||
		stdout.Len() != 0 || stderr.Len() != 0 || len(fake.Calls) != 2 || fake.Calls[1].Args[3] != productionID {
		t.Fatalf("previous switch: code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
	}
}
