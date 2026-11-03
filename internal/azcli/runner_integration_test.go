package azcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRunnerFakeAZRoundTrip(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	fixtures := filepath.Join(dir, "fixtures.json")
	record := filepath.Join(dir, "calls.jsonl")
	data := `[
		{"args":["account","show","space and ünicode"],"stdout":"subscription","stderr":"warning"},
		{"args":["account","set"],"stdout":"partial","stderr":"failure","exitCode":7},
		{"args":["login"],"stdout":"terminal output","stderr":"device code","exitCode":9}
	]`
	if err := os.WriteFile(fixtures, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEAZ_FIXTURES", fixtures)
	t.Setenv("FAKEAZ_RECORD", record)
	t.Setenv("FAKEAZ_ENV_KEYS", "AZCTX_RUNNER_TEST")
	t.Setenv("AZCTX_RUNNER_TEST", "inherited")
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathSeparator)+"absent")
	result, err := runner.Run(context.Background(), []string{accountCommand, showCommand, "space and ünicode"}, azcli.Options{
		Env: []string{"AZCTX_RUNNER_TEST=overridden"},
	})
	if err != nil || result != (azcli.Result{Stdout: subscriptionOutput, Stderr: warningOutput}) {
		t.Fatalf("captured result = %+v, %v", result, err)
	}
	result, err = runner.Run(context.Background(), []string{accountCommand, "set"}, azcli.Options{})
	if err == nil || result != (azcli.Result{Stdout: "partial", Stderr: failureOutput, ExitCode: 7}) {
		t.Fatalf("failed result = %+v, %v", result, err)
	}
	var stdout, stderr bytes.Buffer
	result, err = runner.Run(context.Background(), []string{loginCommand}, azcli.Options{
		Terminal: &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr},
	})
	if err == nil || result != (azcli.Result{Stderr: "device code", ExitCode: 9}) ||
		stdout.String() != "terminal output" || stderr.String() != "device code" {
		t.Fatalf("attached result = %+v, %v, stdout=%q stderr=%q", result, err, stdout.String(), stderr.String())
	}
	assertRecordedCalls(t, record)
	assertStartupErrors(t, runner, dir)
}

func assertStartupErrors(t *testing.T, runner azcli.Runner, dir string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runner.Run(ctx, []string{loginCommand}, azcli.Options{})
	if !errors.Is(err, context.Canceled) || result.ExitCode != -1 {
		t.Fatalf("canceled invocation = %+v, %v", result, err)
	}
	if err := os.Remove(filepath.Join(dir, testutil.ExecutableName("az"))); err != nil {
		t.Fatal(err)
	}
	result, err = runner.Run(context.Background(), []string{loginCommand}, azcli.Options{})
	if err == nil || result.ExitCode != -1 {
		t.Fatalf("startup failure = %+v, %v", result, err)
	}
}

func assertRecordedCalls(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("recorded calls = %s", data)
	}
	var first struct {
		Args []string `json:"args"`
		Env  []string `json:"env"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Args, []string{accountCommand, showCommand, "space and ünicode"}) ||
		!reflect.DeepEqual(first.Env, []string{"AZCTX_RUNNER_TEST=overridden"}) {
		t.Fatalf("first recorded call = %+v", first)
	}
}

func TestRunnerMissingAZ(t *testing.T) {
	t.Setenv("PATH", testutil.WorkDir(t))
	if _, err := azcli.NewRunner(); err == nil {
		t.Fatal("missing az must fail during construction")
	}
}

func TestRunnerCapturesStderrWhenTerminalFails(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	fixtures := filepath.Join(dir, "fixtures.json")
	if err := os.WriteFile(fixtures, []byte(`[{"args":["login"],"stderr":"diagnostic"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKEAZ_FIXTURES", fixtures)
	t.Setenv("FAKEAZ_RECORD", filepath.Join(dir, "calls.jsonl"))
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("terminal unavailable")
	result, err := runner.Run(context.Background(), []string{loginCommand}, azcli.Options{
		Terminal: &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: failedWriter{failure}},
	})
	if !errors.Is(err, failure) || result.Stderr != "diagnostic" || result.ExitCode != 0 {
		t.Fatalf("terminal failure result = %+v, %v", result, err)
	}
}

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }
