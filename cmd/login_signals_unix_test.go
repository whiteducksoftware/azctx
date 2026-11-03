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
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestLoginBinaryForwardsCtrlCAndExits130(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, "az"), childFixturePackage)
	t.Setenv("PATH", dir)
	t.Setenv(childModeEnv, "az-login")
	ready := filepath.Join(dir, "ready")
	t.Setenv(childReadyEnv, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, loginCommand)
	command.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdin, command.Stdout, command.Stderr = strings.NewReader(""), &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	cleanProcess(t, command, cancel)
	waitExecChild(ctx, t, ready)
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := command.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 130 || ctx.Err() != nil || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "child caught interrupt\n") {
		t.Fatalf("login signal lost: error=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
}
