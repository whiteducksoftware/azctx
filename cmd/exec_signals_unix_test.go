//go:build !windows

package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestExecCancellationForwardsInterrupt(t *testing.T) {
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, childFixtureName)
	testutil.Build(t, helper, childFixturePackage)
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	t.Setenv(childModeEnv, childTrapMode)
	ready := filepath.Join(dir, "ready")
	t.Setenv(childReadyEnv, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := cmd.NewRootCommand(deps)
	command.SetContext(ctx)
	command.SetArgs([]string{execSubcommand, productionID, argumentDelimiter, helper})
	var out, messages bytes.Buffer
	command.SetIn(strings.NewReader(""))
	command.SetOut(&out)
	command.SetErr(&messages)
	done := make(chan error, 1)
	go func() { done <- command.Execute() }()
	waitExecChild(ctx, t, ready)
	cancel()
	select {
	case err := <-done:
		if cmd.ExitCode(err) != 130 || !errors.Is(err, context.Canceled) || messages.String() != "child caught interrupt\n" || out.Len() != 0 || len(fake.Calls) != 0 {
			t.Fatalf("error=%v code=%d stdout=%q stderr=%q calls=%+v", err, cmd.ExitCode(err), &out, &messages, fake.Calls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled child did not terminate")
	}
}

func TestExecBinaryForwardsSignalsAndSignalStatuses(t *testing.T) {
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, childFixtureName)
	testutil.Build(t, helper, childFixturePackage)
	binary, _, _ := fakeBinary(t, "[]")
	for _, tc := range []struct {
		name, mode string
		signal     os.Signal
		toChild    bool
		code       int
	}{
		{"CtrlC to parent", childTrapMode, os.Interrupt, false, 130},
		{"SIGTERM to parent", childWaitMode, syscall.SIGTERM, false, 143},
		{"SIGTERM to child", childWaitMode, syscall.SIGTERM, true, 143},
		{"SIGKILL to child", childWaitMode, syscall.SIGKILL, true, 137},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready := filepath.Join(testutil.WorkDir(t), "ready")
			t.Setenv(childModeEnv, tc.mode)
			t.Setenv(childReadyEnv, ready)
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, execSubcommand, developmentID, argumentDelimiter, helper)
			command.WaitDelay = 2 * time.Second
			var out, messages bytes.Buffer
			command.Stdout, command.Stderr = &out, &messages
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			cleanProcess(t, command, cancel)
			pid := waitExecChild(ctx, t, ready)
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Release() }()
			childCompleted := false
			defer func() {
				if !childCompleted {
					_ = child.Kill()
				}
			}()
			target := command.Process
			if tc.toChild {
				target = child
			}
			if err := target.Signal(tc.signal); err != nil {
				t.Fatal(err)
			}
			err = command.Wait()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != tc.code || ctx.Err() != nil || out.Len() != 0 {
				t.Fatalf("error=%v stdout=%q stderr=%q", err, &out, &messages)
			}
			childCompleted = true
			want := ""
			if tc.mode == childTrapMode {
				want = "child caught interrupt\n"
			}
			if messages.String() != want {
				t.Fatalf("child streams contaminated: %q", &messages)
			}
		})
	}
}
