//go:build darwin || linux

package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestProductionTerminalInterruptDeliveredOnce(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, "az")
	testutil.Build(t, helper, childFixturePackage)
	t.Setenv("PATH", dir)
	t.Setenv("AZCTX_TEST_CHILD_READ_LINE", "1")
	for _, mode := range []string{countInterruptMode, azExitOneMode, ignoreSignalMode} {
		t.Run(mode, func(t *testing.T) {
			ready := filepath.Join(testutil.WorkDir(t), "ready")
			started := filepath.Join(filepath.Dir(ready), "started")
			t.Setenv(childModeEnv, mode)
			t.Setenv(childReadyEnv, ready)
			t.Setenv("AZCTX_TEST_CHILD_STARTED", started)
			args := []string{loginCommand}
			if mode == countInterruptMode {
				args = []string{execSubcommand, developmentID, argumentDelimiter, helper}
			}
			master, slave := openContractPTY(t)
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			process := exec.CommandContext(ctx, binary, args...)
			var stdout bytes.Buffer
			process.Stdin, process.Stdout, process.Stderr = slave, &stdout, slave
			process.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = process.Process.Kill() }()
			_ = slave.Close()
			screen := make(chan string, 1)
			go func() {
				data, _ := io.ReadAll(master)
				screen <- string(data)
			}()
			pid := waitExecChild(ctx, t, started)
			defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
			if _, err := master.Write([]byte("hello\n")); err != nil {
				t.Fatal(err)
			}
			waitExecChild(ctx, t, ready)
			if _, err := master.Write([]byte{3}); err != nil {
				t.Fatal(err)
			}
			err := process.Wait()
			_ = master.Close()
			output := <-screen
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 130 || ctx.Err() != nil || stdout.Len() != 0 {
				t.Fatalf("terminal interruption: err=%v stdout=%q screen=%q context=%v", err, &stdout, output, ctx.Err())
			}
			if !strings.Contains(output, "child read hello") {
				t.Fatalf("child could not read attached foreground stdin: %q", output)
			}
			if mode == countInterruptMode && strings.Count(output, "child caught") != 1 {
				t.Fatalf("terminal signal delivered twice: %q", output)
			}
		})
	}
}
