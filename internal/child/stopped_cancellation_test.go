//go:build darwin || linux

package child_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/child"
)

const stoppedChildEnv = "AZCTX_TEST_STOPPED_CHILD"

func TestRunCancellationResumesStoppedChild(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	lines := make(chan string, 4)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	done := make(chan struct{}, 1)
	var status int
	var runErr error
	var diagnostics bytes.Buffer
	go func() {
		status, runErr = child.Run(runCtx, self, []string{"-test.run=^TestStoppedCancellationHelper$"},
			[]string{stoppedChildEnv + "=1"}, child.Streams{Stdout: writer, Stderr: &diagnostics})
		_ = writer.Close()
		done <- struct{}{}
	}()
	completed := false
	defer reapCanceledRun(t, cancelRun, done, &completed)
	pid := stoppedChildPID(ctx, t, lines)
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	waitStoppedProcess(ctx, t, pid)
	cancelRun()
	select {
	case <-done:
		completed = true
	case <-ctx.Done():
		t.Fatal("stopped cancellation did not finish")
	}
	if status != 130 || !errors.Is(runErr, context.Canceled) {
		t.Fatalf("stopped cancellation status=%d error=%v", status, runErr)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("stopped target survived cancellation: %v", err)
	}
	select {
	case line := <-lines:
		if line != "stopped child caught interrupt" {
			t.Fatalf("stopped cancellation acknowledgement=%q diagnostics=%s", line, &diagnostics)
		}
	case <-ctx.Done():
		t.Fatalf("stopped child was killed without handling its pending interrupt: %s", &diagnostics)
	}
}

func stoppedChildPID(ctx context.Context, t *testing.T, lines <-chan string) int {
	t.Helper()
	select {
	case line := <-lines:
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 {
			t.Fatalf("invalid stopped child ready event %q: %v", line, err)
		}
		return pid
	case <-ctx.Done():
		t.Fatal("child did not acknowledge startup")
		return 0
	}
}

func reapCanceledRun(t *testing.T, cancel context.CancelFunc, done <-chan struct{}, completed *bool) {
	t.Helper()
	cancel()
	if *completed {
		return
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("cancellation helper was not reaped during cleanup")
	}
}

func waitStoppedProcess(ctx context.Context, t *testing.T, pid int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Observe the kernel's stopped state, not just the pre-stop ready event.
		state, err := exec.CommandContext(ctx, "/bin/ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // Test helper PID from its explicit startup event; no shell or user input.
		if err != nil {
			t.Fatalf("observe stopped child: %v", err)
		}
		if strings.HasPrefix(strings.TrimSpace(string(state)), "T") {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("child never stopped")
		}
	}
}

func TestStoppedCancellationHelper(_ *testing.T) {
	if os.Getenv(stoppedChildEnv) == "" {
		return
	}
	interrupts := make(chan os.Signal, 8)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGCONT)
	fmt.Fprintln(os.Stdout, os.Getpid())
	if err := syscall.Kill(os.Getpid(), syscall.SIGSTOP); err != nil {
		os.Exit(2)
	}
	for value := range interrupts {
		if value == os.Interrupt {
			break
		}
	}
	fmt.Fprintln(os.Stdout, "stopped child caught interrupt")
	os.Exit(0)
}
