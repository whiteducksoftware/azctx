package cmd_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBinaryLoginSucceedsWithInheritedStderrPipe(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("az")), childFixturePackage)
	ready := filepath.Join(dir, "ready")
	t.Setenv("PATH", dir)
	t.Setenv(childModeEnv, "az-pipe")
	t.Setenv(childReadyEnv, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, loginCommand)
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdin, command.Stdout, command.Stderr = strings.NewReader(""), &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	cleanProcess(t, command, cancel)
	pid := waitExecChild(ctx, t, ready)
	descendant, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = descendant.Release() }()
	defer func() { _ = descendant.Kill() }()
	if err := command.Wait(); err != nil || ctx.Err() != nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Successfully fetched") {
		t.Fatalf("login failed after successful child exit: error=%v stdout=%q stderr=%q context=%v", err, &stdout, &stderr, ctx.Err())
	}
}
