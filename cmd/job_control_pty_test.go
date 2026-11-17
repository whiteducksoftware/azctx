//go:build darwin || linux

package cmd_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	jobControlEnv     = "AZCTX_TEST_JOB_CONTROL"
	jobNativeEnv      = "AZCTX_TEST_JOB_NATIVE_READER"
	jobPrompt         = "AZCTX_JOB_PROMPT> "
	nativeCommandFlag = "-c"
)

func TestProductionShellJobSuspendResume(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
		suspendInteractiveJob(ctx, t, master, screen)
		offset := len(screen.String())
		writeJobInput(t, master, "fg\n")
		waitJobOutput(ctx, t, screen, offset, "JOB_RESUMED")
		writeJobInput(t, master, "after-resume\n")
		screen.waitFor(ctx, t, "JOB_INPUT=after-resume")
		writeJobInput(t, master, "\x03")
		screen.waitFor(ctx, t, "JOB_INTERRUPT=1")
		finishInteractiveJob(ctx, t, master, screen, 130)
		screen.waitFor(ctx, t, "JOB_INTERRUPTS=1")
		if strings.Contains(screen.String(), "JOB_INTERRUPT=2") {
			t.Fatalf("terminal interrupt delivered twice: %s", screen)
		}
	})
}

func TestProductionShellJobResumeExitStatus(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
		for range 2 {
			suspendInteractiveJob(ctx, t, master, screen)
			offset := len(screen.String())
			writeJobInput(t, master, "fg\n")
			waitJobOutput(ctx, t, screen, offset, "JOB_RESUMED")
			writeJobInput(t, master, "after-resume\n")
			waitJobOutput(ctx, t, screen, offset, "JOB_INPUT=after-resume")
		}
		finishInteractiveJob(ctx, t, master, screen, 37)
	})
}

func TestProductionShellJobBackgroundReadStops(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
		suspendInteractiveJob(ctx, t, master, screen)
		offset := len(screen.String())
		writeJobInput(t, master, "bg\n")
		waitJobOutput(ctx, t, screen, offset, "Stopped")
		offset = len(screen.String())
		writeJobInput(t, master, "fg\n")
		waitJobOutput(ctx, t, screen, offset, "JOB_RESUMED")
		writeJobInput(t, master, "after-bg\n")
		waitJobOutput(ctx, t, screen, offset, "JOB_INPUT=after-bg")
		finishInteractiveJob(ctx, t, master, screen, 37)
	})
}

func TestProductionShellJobKilledWhileStopped(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
		suspendInteractiveJob(ctx, t, master, screen)
		group := recordedJobGroup(t, os.Getenv(jobControlEnv))
		if err := syscall.Kill(-group, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		offset := len(screen.String())
		writeJobInput(t, master, "fg\n")
		waitJobOutput(ctx, t, screen, offset, jobPrompt)
		readInteractiveStatus(ctx, t, master, screen, 137)
	})
}

func TestProductionShellJobNativeResume(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	t.Setenv(jobNativeEnv, "1")
	runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
		screen.waitFor(ctx, t, "JOB_READY")
		writeJobInput(t, master, "before-stop\n")
		screen.waitFor(ctx, t, "NATIVE_INPUT=before-stop")
		offset := len(screen.String())
		writeJobInput(t, master, "\x1a")
		waitJobOutput(ctx, t, screen, offset, "Stopped")
		waitJobOutput(ctx, t, screen, offset, jobPrompt)
		writeJobInput(t, master, "fg\nafter-resume\n")
		screen.waitFor(ctx, t, "NATIVE_INPUT=after-resume")
		offset = len(screen.String())
		writeJobInput(t, master, "finish\n")
		waitJobOutput(ctx, t, screen, offset, jobPrompt)
		readInteractiveStatus(ctx, t, master, screen, 0)
	})
}

func TestProductionShellStoppedCancellation(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	for _, cooperative := range []bool{true, false} {
		t.Run(fmt.Sprintf("cooperative=%t", cooperative), func(t *testing.T) {
			runInteractiveJob(t, binary, func(ctx context.Context, master *os.File, screen *ptyScreen) {
				suspendInteractiveJob(ctx, t, master, screen)
				offset := len(screen.String())
				start := time.Now()
				// Signals to a stopped shell job are pending until it resumes.
				// No second Ctrl+C is needed to resume/cancel the stopped child.
				writeJobInput(t, master, "kill -INT %1; fg\n")
				waitJobOutput(ctx, t, screen, offset, "JOB_INTERRUPT=1")
				if cooperative {
					finishInteractiveJob(ctx, t, master, screen, 130)
					screen.waitFor(ctx, t, "JOB_INTERRUPTS=1")
				} else {
					waitJobOutput(ctx, t, screen, offset, jobPrompt)
					readInteractiveStatus(ctx, t, master, screen, 130)
					if elapsed := time.Since(start); elapsed > 5*time.Second {
						t.Fatalf("stopped cancellation exceeded kill deadline: %s", elapsed)
					}
				}
				if strings.Contains(screen.String(), "JOB_INTERRUPT=2") {
					t.Fatalf("stopped cancellation delivered twice: %s", screen)
				}
			})
		})
	}
}

func suspendInteractiveJob(ctx context.Context, t *testing.T, master *os.File, screen *ptyScreen) {
	t.Helper()
	screen.waitFor(ctx, t, "JOB_READY")
	offset := len(screen.String())
	writeJobInput(t, master, "before-stop\n")
	waitJobOutput(ctx, t, screen, offset, "JOB_INPUT=before-stop")
	offset = len(screen.String())
	writeJobInput(t, master, "\x1a")
	waitJobOutput(ctx, t, screen, offset, "Stopped")
	waitJobOutput(ctx, t, screen, offset, jobPrompt)
}

func runInteractiveJob(t *testing.T, binary string, exercise func(context.Context, *os.File, *ptyScreen)) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	work := testutil.WorkDir(t)
	wrapper, group := filepath.Join(work, "wrapper"), filepath.Join(work, "child-group")
	t.Setenv(jobControlEnv, group)
	t.Setenv("PS1", jobPrompt)
	t.Setenv("PS2", "")
	t.Setenv("PROMPT_COMMAND", "")
	t.Setenv("BASH_ENV", "")
	t.Setenv("BASH_SILENCE_DEPRECATION_WARNING", "1")
	t.Setenv("ENV", "")
	t.Setenv("LC_ALL", "C")
	t.Setenv("TERM", "dumb")
	master, slave := openContractPTY(t)
	if os.Getenv(jobNativeEnv) != "" {
		state, err := unix.IoctlGetTermios(int(slave.Fd()), terminalGetRequest)
		if err != nil {
			t.Fatal(err)
		}
		state.Lflag &^= unix.ECHO
		if err := unix.IoctlSetTermios(int(slave.Fd()), terminalSetRequest, state); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	shell := exec.CommandContext(ctx, "/bin/bash", "--noprofile", "--norc", "-i")
	shell.Stdin, shell.Stdout, shell.Stderr = slave, slave, slave
	shell.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		killRecordedProcess(group, true)
		killRecordedProcess(wrapper, true)
		_ = shell.Process.Kill()
		_ = shell.Wait()
	}()
	_ = slave.Close()
	screen := recordPTY(master)
	screen.waitFor(ctx, t, jobPrompt)
	offset := len(screen.String())
	writeJobInput(t, master, "set -b\n")
	waitJobOutput(ctx, t, screen, offset, jobPrompt)
	// The exec'd shell records the shell-visible job PID for bounded cleanup,
	// including the RED case where Ctrl+Z never returns a prompt.
	command := "printf '%s' \"$$\" > " + quoteJobArg(wrapper) + "; exec " + quoteJobArg(binary) +
		" exec " + developmentID + " -- " + quoteJobArg(self) + " -test.run=^TestProductionJobControlHelper$"
	writeJobInput(t, master, "/bin/bash -c "+quoteJobArg(command)+"\n")
	exercise(ctx, master, screen)
	for _, path := range []string{wrapper, group} {
		pid := recordedJobGroup(t, path)
		if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("job group %d survived shell completion: %v", pid, err)
		}
	}
}

func recordedJobGroup(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned process-group cleanup record, never user data.
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid job group record %q: %v", data, err)
	}
	return pid
}

func quoteJobArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeJobInput(t *testing.T, master *os.File, input string) {
	t.Helper()
	if _, err := master.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func waitJobOutput(ctx context.Context, t *testing.T, screen *ptyScreen, offset int, marker string) {
	t.Helper()
	for !strings.Contains(screen.String()[offset:], marker) {
		select {
		case <-screen.updated:
		case <-screen.done:
			t.Fatalf("terminal closed before %q: %s", marker, screen)
		case <-ctx.Done():
			t.Fatalf("terminal never displayed %q: %s", marker, screen)
		}
	}
}

func finishInteractiveJob(ctx context.Context, t *testing.T, master *os.File, screen *ptyScreen, status int) {
	t.Helper()
	offset := len(screen.String())
	writeJobInput(t, master, "finish\n")
	waitJobOutput(ctx, t, screen, offset, jobPrompt)
	readInteractiveStatus(ctx, t, master, screen, status)
}

func readInteractiveStatus(ctx context.Context, t *testing.T, master *os.File, screen *ptyScreen, status int) {
	t.Helper()
	offset := len(screen.String())
	writeJobInput(t, master, "printf 'JOB_STATUS=%s\\n' \"$?\"\n")
	waitJobOutput(ctx, t, screen, offset, fmt.Sprintf("JOB_STATUS=%d", status))
}

func TestProductionJobControlHelper(t *testing.T) {
	record := os.Getenv(jobControlEnv)
	if record == "" {
		return
	}
	if err := os.WriteFile(record, []byte(strconv.Itoa(syscall.Getpgrp())), 0o600); err != nil { //nolint:gosec // Test-owned process-group cleanup record, never user data.
		t.Fatal(err)
	}
	if os.Getenv(jobNativeEnv) != "" {
		// exec resets caught signals: the reader uses native default SIGCONT
		// disposition, not the Go helper's resume notification handler.
		fmt.Fprintln(os.Stdout, "JOB_READY")
		const reader = `while :; do if IFS= read -r line; then printf 'NATIVE_INPUT=%s\n' "$line"; if [ "$line" = finish ]; then exit 0; fi; fi; done`
		if err := syscall.Exec("/bin/sh", []string{"sh", nativeCommandFlag, reader}, os.Environ()); err != nil { //nolint:gosec // Literal native offline helper; no user-controlled command.
			t.Fatal(err)
		}
	}
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, os.Interrupt, syscall.SIGCONT)
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	fmt.Fprintln(os.Stdout, "JOB_READY")
	interrupts := 0
	for {
		select {
		case value := <-signals:
			if value == syscall.SIGCONT {
				fmt.Fprintln(os.Stdout, "JOB_RESUMED")
			} else {
				interrupts++
				fmt.Fprintf(os.Stdout, "JOB_INTERRUPT=%d\n", interrupts)
			}
		case line := <-lines:
			fmt.Fprintf(os.Stdout, "JOB_INPUT=%s\n", line)
			if line == "finish" {
				fmt.Fprintf(os.Stdout, "JOB_INTERRUPTS=%d\n", interrupts)
				if interrupts != 0 {
					os.Exit(1)
				}
				os.Exit(37)
			}
		}
	}
}
