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

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	countInterruptMode = "count"
	azExitOneMode      = "az-one"
	ignoreSignalMode   = "ignore"
	missingStateCase   = "switch missing state"
)

func TestProductionInterrupts(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, "az")
	testutil.Build(t, helper, childFixturePackage)
	t.Setenv("PATH", dir)
	for _, tc := range []struct {
		name, mode string
		args       []string
		group      bool
		second     bool
	}{
		{"group interrupt once", countInterruptMode, []string{execSubcommand, developmentID, argumentDelimiter, helper}, true, false},
		{"login exits one", azExitOneMode, []string{loginCommand}, false, false},
		{"group login exits one", azExitOneMode, []string{loginCommand}, true, false},
		{"ignored interrupt", ignoreSignalMode, []string{loginCommand}, false, false},
		{"second interrupt", ignoreSignalMode, []string{loginCommand}, false, true},
		{"MFA stops", "az-mfa-one", []string{loginCommand, allTenantsFlag}, true, false},
		{"switch rolls back", azExitOneMode, []string{developmentID}, true, false},
		{missingStateCase, azExitOneMode, []string{developmentID}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := testutil.WorkDir(t)
			t.Setenv("AZCTX_CONFIG_DIR", work)
			const original = "{ \"previousSubscriptionId\": \"" + developmentID + "\", \"recentSubscriptionIds\": [\"" + productionID + "\"] }\n"
			statePath := filepath.Join(work, "state.json")
			if tc.name != missingStateCase {
				writeFixture(t, statePath, original)
			}
			ready, record := filepath.Join(work, "ready"), filepath.Join(work, "calls")
			t.Setenv(childModeEnv, tc.mode)
			t.Setenv(childReadyEnv, ready)
			t.Setenv("AZCTX_TEST_CHILD_RECORD", record)
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			process := exec.CommandContext(ctx, binary, tc.args...) //nolint:gosec // Offline test binary and table-owned argv, no shell.
			process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			process.WaitDelay = time.Second
			process.Stdin = strings.NewReader("y\n")
			var stdout, stderr bytes.Buffer
			process.Stdout, process.Stderr = &stdout, &stderr
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL) }()
			pid := waitExecChild(ctx, t, ready)
			defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
			start := time.Now()
			if tc.group {
				if err := syscall.Kill(-process.Process.Pid, syscall.SIGINT); err != nil {
					t.Fatal(err)
				}
			} else if err := process.Process.Signal(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			if tc.second {
				time.Sleep(150 * time.Millisecond)
				if err := process.Process.Signal(os.Interrupt); err != nil {
					t.Fatal(err)
				}
			}
			err := process.Wait()
			assertProductionInterrupted(ctx, t, err, &stdout, &stderr, tc.mode)
			if tc.second && time.Since(start) > 1500*time.Millisecond {
				t.Fatalf("second interrupt did not kill promptly: %s", time.Since(start))
			}

			if tc.mode == "az-mfa-one" {
				data, err := os.ReadFile(record)
				if err != nil || string(data) != "xx" {
					t.Fatalf("MFA continued after interruption: calls=%q err=%v stderr=%q", data, err, &stderr)
				}
			}
			assertInterruptedState(t, statePath, original, tc.name == missingStateCase)
		})
	}
}

func assertInterruptedState(t *testing.T, path, original string, missing bool) {
	t.Helper()
	after, err := os.ReadFile(path)
	if missing {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("interrupted switch created state: %q err=%v", after, err)
		}
	} else if err != nil || string(after) != original {
		t.Fatalf("rollback changed exact state bytes: %q err=%v", after, err)
	}
}

func assertProductionInterrupted(ctx context.Context, t *testing.T, err error, stdout, stderr *bytes.Buffer, mode string) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 130 || ctx.Err() != nil || stdout.Len() != 0 {
		t.Fatalf("interrupt: err=%v stdout=%q stderr=%q context=%v", err, stdout, stderr, ctx.Err())
	}
	if mode == countInterruptMode && strings.Count(stderr.String(), "child caught") != 1 {
		t.Fatalf("group signal delivered twice: %q", stderr)
	}
}
