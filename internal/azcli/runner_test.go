package azcli_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	accountCommand     = "account"
	showCommand        = "show"
	loginCommand       = "login"
	subscriptionOutput = "subscription"
	failureOutput      = "failure"
	warningOutput      = "warning"
)

func TestFakeRunnerRecordsAndReplays(t *testing.T) {
	failure := errors.New("exit status 7")
	fake := &testutil.FakeRunner{Responses: []testutil.Response{
		{Result: azcli.Result{Stdout: subscriptionOutput, Stderr: warningOutput}},
		{Result: azcli.Result{Stderr: failureOutput, ExitCode: 7}, Err: failure},
	}}
	var runner azcli.Runner = fake
	args := []string{accountCommand, showCommand}
	env := []string{"AZURE_CONFIG_DIR=fixture"}
	first, err := runner.Run(context.Background(), args, azcli.Options{Env: env})
	if err != nil || first.Stdout != subscriptionOutput || first.Stderr != warningOutput || first.ExitCode != 0 {
		t.Fatalf("captured result = %+v, %v", first, err)
	}
	args[0], env[0] = "changed", "changed"
	var stdout, stderr bytes.Buffer
	second, err := runner.Run(context.Background(), []string{loginCommand}, azcli.Options{
		Terminal: &azcli.Terminal{Stdout: &stdout, Stderr: &stderr},
	})
	if !errors.Is(err, failure) || second.ExitCode != 7 || stderr.String() != failureOutput {
		t.Fatalf("attached result = %+v, %v, stderr = %q", second, err, stderr.String())
	}
	if !reflect.DeepEqual(fake.Calls[0].Args, []string{accountCommand, showCommand}) ||
		!reflect.DeepEqual(fake.Calls[0].Env, []string{"AZURE_CONFIG_DIR=fixture"}) ||
		fake.Calls[0].Attached || !fake.Calls[1].Attached {
		t.Fatalf("recorded calls = %+v", fake.Calls)
	}
	if _, err := runner.Run(context.Background(), nil, azcli.Options{}); err == nil {
		t.Fatal("an unexpected call must fail instead of silently succeeding")
	}
}
