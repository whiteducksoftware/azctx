package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBrokerFakeAZConfirmedAttachedInput(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	for _, platform := range []string{platformWindows, platformMac} {
		for _, action := range []string{brokerEnable, brokerDisable} {
			t.Run(platform+"/"+action, func(t *testing.T) {
				testBrokerAttached(t, dir, platform, action)
			})
		}
	}
}

func testBrokerAttached(t *testing.T, dir, platform, action string) {
	t.Helper()
	key, value := brokerWindows, "true"
	if platform == platformMac {
		key = brokerMac
	}
	if action == brokerDisable {
		value = boolFalse
	}
	deps := commandDependencies(t, &testutil.FakeRunner{})
	deps.Platform, deps.FS = platform, fstest.MapFS{}
	t.Setenv("PATH", dir)
	fixturePath, record := filepath.Join(dir, platform+"-"+action+".json"), filepath.Join(dir, platform+"-"+action+".jsonl")
	want := [][]string{{versionCommand, outputFlag, jsonFormat}, {accountCommand, "clear"}, {azureConfigFile, setCommand, "core." + key + "=" + value}, {loginCommand}}
	fixtures := []map[string]any{
		{fixtureArgs: want[0], "stdout": brokerVersionJSON},
		{fixtureArgs: want[1]},
		{fixtureArgs: want[2]},
		{fixtureArgs: want[3], "stdout": "attached stdout\n", "stderr": "attached stderr\n", "waitForInput": true},
	}
	data, err := json.Marshal(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, fixturePath, string(data))
	t.Setenv("FAKEAZ_FIXTURES", fixturePath)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("FAKEAZ_ENV_KEYS", "AZURE_CORE_LOGIN_EXPERIENCE_V2")
	t.Setenv("AZURE_CORE_LOGIN_EXPERIENCE_V2", "on")
	deps.Runner, err = azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cmd.Run([]string{authCommand, brokerCommand, action}, deps, strings.NewReader("yes\nAzure login input\n"), &stdout, &stderr)
	calls, err := os.ReadFile(record)
	if code != 0 || err != nil || strings.Count(string(calls), "\n") != 4 ||
		stdout.String() != "attached stdout\n" || !strings.Contains(stderr.String(), "attached stderr") {
		t.Fatalf("code=%d error=%v calls=%s stdout=%q stderr=%q", code, err, calls, stdout.String(), stderr.String())
	}
	for index, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		var call testutil.Call
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatal(err)
		}
		env := "AZURE_CORE_LOGIN_EXPERIENCE_V2=on"
		if index == 3 {
			env = childPickerOff
		}
		if !reflect.DeepEqual(call.Args, want[index]) || !reflect.DeepEqual(call.Env, []string{env}) {
			t.Fatalf("call %d=%+v", index, call)
		}
	}
}
