package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	fixtureStdout = "stdout"
	fixtureStderr = "stderr"
)

func TestMFAFakeAZConfirmationPreservesAttachedInputAndStreams(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	deps := commandDependencies(t, &testutil.FakeRunner{})
	t.Setenv("PATH", dir)
	fixturePath, record := filepath.Join(dir, "mfa.json"), filepath.Join(dir, "mfa.jsonl")
	want := [][]string{
		{versionCommand, outputFlag, jsonFormat},
		{loginCommand, deviceCodeFlag},
		{loginCommand, tenantFlag, mfaTenant, deviceCodeFlag},
		{loginCommand, tenantFlag, registrationTenant, deviceCodeFlag},
	}
	const deviceInstructions = "Use the device code SYNTHETIC at https://microsoft.com/devicelogin\n"
	fixtures := []map[string]any{
		{fixtureArgs: want[0], fixtureStdout: brokerVersionJSON},
		{fixtureArgs: want[1], fixtureStdout: deviceInstructions, fixtureStderr: mfaFixture(t, "50076") + mfaFixture(t, "50079")},
		{fixtureArgs: want[2], fixtureStdout: "first retry stdout\n"},
		{fixtureArgs: want[3], fixtureStderr: "last retry stderr\n", "waitForInput": true},
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
	code := cmd.Run([]string{loginCommand, deviceCodeFlag}, deps, attachedMFAInput(t), &stdout, &stderr)
	calls, err := os.ReadFile(record)
	if code != 0 || err != nil || stdout.String() != deviceInstructions+"first retry stdout\n" ||
		strings.Count(string(calls), "\n") != 4 || strings.Count(stderr.String(), "[Y/n]") != 1 ||
		!strings.Contains(stderr.String(), "last retry stderr") || !strings.Contains(stderr.String(), "MFA registration required") {
		t.Fatalf("code=%d error=%v calls=%s stdout=%q stderr=%q", code, err, calls, stdout.String(), stderr.String())
	}
	for index, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		var call testutil.Call
		if err := json.Unmarshal([]byte(line), &call); err != nil {
			t.Fatal(err)
		}
		env := "AZURE_CORE_LOGIN_EXPERIENCE_V2=on"
		if index != 0 {
			env = childPickerOff
		}
		if !reflect.DeepEqual(call.Args, want[index]) || !reflect.DeepEqual(call.Env, []string{env}) {
			t.Fatalf("call %d=%+v", index, call)
		}
	}
}

func attachedMFAInput(t *testing.T) *os.File {
	t.Helper()
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if _, err := writer.WriteString("yes\ninput for final Azure child\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestMFARetryReplacesPassthroughTenantSelector(t *testing.T) {
	for _, selector := range [][]string{
		{tenantFlag, tenantID},
		{"--tenant=" + tenantID},
		{"-t", tenantID},
		{"-t" + tenantID},
		{"-t=" + tenantID},
	} {
		fake := &testutil.FakeRunner{Responses: []testutil.Response{
			{Result: azcli.Result{Stdout: brokerVersionJSON}},
			{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
			{},
		}}
		deps := commandDependencies(t, fake)
		var stdout, stderr bytes.Buffer
		args := append([]string{loginCommand, argumentDelimiter}, selector...)
		if code := cmd.Run(args, deps, strings.NewReader("yes\n"), &stdout, &stderr); code != 0 ||
			len(fake.Calls) != 3 || !reflect.DeepEqual(fake.Calls[2].Args, []string{loginCommand, tenantFlag, mfaTenant}) {
			t.Fatalf("code=%d calls=%+v stdout=%q stderr=%q", code, fake.Calls, stdout.String(), stderr.String())
		}
	}
}
