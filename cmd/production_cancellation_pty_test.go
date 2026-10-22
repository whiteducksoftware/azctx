//go:build darwin || linux

package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/whiteducksoftware/azctx/internal/child"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	shellModelEnv      = "AZCTX_TEST_SHELL_MODEL"
	jobForeground      = "foreground"
	jobNoncontrolling  = "noncontrolling"
	terminalStateMode  = "disposition"
	terminalIgnoredEnv = "AZCTX_TEST_TERMINAL_STOP_IGNORED"
)

type shellJobCase struct {
	name, job, mode string
	tostop          bool
	interrupt       bool
	code            int
}

// Unlike a session-leader azctx, a shell's foreground job is not an orphaned
// process group: background terminal operations can really stop it with SIGTTOU.
func TestProductionShellJobTerminal(t *testing.T) {
	binary, _, _ := fakeBinary(t, "[]")
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, "az")
	testutil.Build(t, helper, childFixturePackage)
	t.Setenv("PATH", dir)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []shellJobCase{
		{"foreground restore", jobForeground, countInterruptMode, false, true, 130},
		{"TOSTOP stderr", jobForeground, countInterruptMode, true, true, 130},
		{"TOSTOP login", jobForeground, azExitOneMode, true, true, 130},
		{"noncontrolling terminal", jobNoncontrolling, "exit", false, false, 37},
		{"background wrapper", "background", "exit", false, false, 37},
		{"child signal disposition", jobForeground, terminalStateMode, false, false, 0},
		{"prior ignored disposition", jobForeground, terminalStateMode, false, false, 0},
		{"prior notification", jobForeground, terminalStateMode, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(terminalIgnoredEnv, "")
			if tc.name == "prior ignored disposition" {
				t.Setenv(terminalIgnoredEnv, "ignore")
			} else if tc.name == "prior notification" {
				t.Setenv(terminalIgnoredEnv, "notify")
			}
			runShellJob(t, binary, self, helper, tc)
		})
	}
}

func runShellJob(t *testing.T, binary, self, helper string, tc shellJobCase) {
	t.Helper()
	work := testutil.WorkDir(t)
	ready, started := filepath.Join(work, "ready"), filepath.Join(work, "started")
	t.Setenv(shellModelEnv, tc.job)
	t.Setenv(childModeEnv, tc.mode)
	t.Setenv(childReadyEnv, ready)
	t.Setenv("AZCTX_TEST_CHILD_STARTED", started)
	jobPID := filepath.Join(work, "job-pid")
	t.Setenv("AZCTX_TEST_SHELL_JOB_PID", jobPID)
	t.Cleanup(func() {
		killRecordedProcess(jobPID, true)
		killRecordedProcess(started, false)
	})
	t.Setenv("AZCTX_TEST_CHILD_READ_LINE", "")
	if tc.interrupt {
		t.Setenv("AZCTX_TEST_CHILD_READ_LINE", "1")
	}
	args := []string{execSubcommand, developmentID, argumentDelimiter, helper}
	if tc.mode == terminalStateMode {
		args = []string{execSubcommand, developmentID, argumentDelimiter, self, "-test.run=^TestProductionShellSignalHelper$"}
	}
	if tc.mode == azExitOneMode {
		args = []string{loginCommand}
	}
	master, slave := openContractPTY(t)
	if tc.tostop {
		enableTerminalTOSTOP(t, slave)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	shell := exec.CommandContext(ctx, self, append([]string{"-test.run=^TestProductionShellModelHelper$", argumentDelimiter, binary}, args...)...) //nolint:gosec // Re-exec this test binary with an offline binary and literal test-owned arguments.
	shell.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	var result bytes.Buffer
	shell.Stdin, shell.Stdout, shell.Stderr = slave, &result, slave
	if err := shell.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		_ = shell.Process.Kill()
		if !waited {
			_ = shell.Wait()
		}
	}()
	_ = slave.Close()
	screen := recordPTY(master)
	if tc.interrupt {
		interruptShellChild(ctx, t, master, screen, started, ready)
	}
	err := shell.Wait()
	waited = true
	_ = master.Close()
	output := screen.output()
	want := fmt.Sprintf("job exit=%d restored=true", tc.code)
	if err != nil || ctx.Err() != nil || !strings.Contains(result.String(), want) {
		t.Fatalf("shell job: err=%v context=%v result=%q screen=%q", err, ctx.Err(), &result, output)
	}
	assertShellOutput(t, tc, output)
}

func enableTerminalTOSTOP(t *testing.T, slave *os.File) {
	t.Helper()
	state, err := unix.IoctlGetTermios(int(slave.Fd()), terminalGetRequest)
	if err != nil {
		t.Fatal(err)
	}
	state.Lflag |= unix.TOSTOP
	if err := unix.IoctlSetTermios(int(slave.Fd()), terminalSetRequest, state); err != nil {
		t.Fatal(err)
	}
}

func killRecordedProcess(path string, group bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return
	}
	if group {
		pid = -pid
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// ptyScreen drains a PTY master and records everything the terminal displayed.
type ptyScreen struct {
	mu      sync.Mutex
	data    []byte
	updated chan struct{}
	done    chan struct{}
}

func recordPTY(master *os.File) *ptyScreen {
	screen := &ptyScreen{updated: make(chan struct{}, 1), done: make(chan struct{})}
	go func() {
		defer close(screen.done)
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			screen.mu.Lock()
			screen.data = append(screen.data, buffer[:n]...)
			screen.mu.Unlock()
			select {
			case screen.updated <- struct{}{}:
			default:
			}
			if err != nil {
				return
			}
		}
	}()
	return screen
}

func (s *ptyScreen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.data)
}

// waitFor returns once the reader has drained marker from the terminal output queue.
func (s *ptyScreen) waitFor(ctx context.Context, t *testing.T, marker string) {
	t.Helper()
	for !strings.Contains(s.String(), marker) {
		select {
		case <-s.updated:
		case <-s.done:
			if !strings.Contains(s.String(), marker) {
				t.Fatalf("terminal closed before %q: %q", marker, s)
			}
		case <-ctx.Done():
			t.Fatalf("terminal never displayed %q: %q", marker, s)
		}
	}
}

func (s *ptyScreen) output() string {
	<-s.done
	return s.String()
}

// interruptShellChild sends Ctrl-C only after the screen reader has received the
// child's input acknowledgement. Without NOFLSH, VINTR discards unread terminal
// output, so the ready file and the input echo cannot prove the acknowledgement
// survives the interrupt.
func interruptShellChild(ctx context.Context, t *testing.T, master *os.File, screen *ptyScreen, started, ready string) {
	t.Helper()
	pid := waitExecChild(ctx, t, started)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if _, err := master.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	waitExecChild(ctx, t, ready)
	screen.waitFor(ctx, t, "child read hello")
	if _, err := master.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
}

func assertShellOutput(t *testing.T, tc shellJobCase, output string) {
	t.Helper()
	if tc.mode == countInterruptMode && (!strings.Contains(output, "child read hello") || strings.Count(output, "child caught") != 1) {
		t.Fatalf("foreground input/interrupt: %q", output)
	}
	if tc.mode == terminalStateMode {
		if !strings.Contains(output, "parent SIGTTOU preserved=true") || (os.Getenv(terminalIgnoredEnv) == "" && !strings.Contains(output, "child SIGTTOU ignored=false")) {
			t.Fatalf("child inherited changed terminal-stop disposition: %q", output)
		}
	} else if !tc.interrupt && !strings.Contains(output, "child stderr") {
		t.Fatalf("wrapper lost attached stderr: %q", output)
	}
}

func TestProductionShellSignalHelper(t *testing.T) {
	if os.Getenv(shellModelEnv) == "" {
		return
	}
	var notified chan os.Signal
	if os.Getenv(terminalIgnoredEnv) == "ignore" {
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
	} else if os.Getenv(terminalIgnoredEnv) == "notify" {
		notified = make(chan os.Signal, 1)
		signal.Notify(notified, syscall.SIGTTOU)
		defer signal.Stop(notified)
	}
	before := signal.Ignored(syscall.SIGTTOU)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	code, err := child.Run(context.Background(), self, []string{"-test.run=^TestProductionShellSignalLeaf$"}, nil,
		child.Streams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr})
	if err != nil || code != 0 {
		t.Fatalf("nested attached command: code=%d error=%v", code, err)
	}
	if notified != nil {
		if err := syscall.Kill(os.Getpid(), syscall.SIGTTOU); err != nil {
			t.Fatal(err)
		}
		select {
		case <-notified:
		case <-time.After(time.Second):
			t.Fatal("attached child removed an existing SIGTTOU subscription")
		}
	}
	fmt.Fprintf(os.Stderr, "parent SIGTTOU preserved=%t\n", signal.Ignored(syscall.SIGTTOU) == before)
}

func TestProductionShellSignalLeaf(_ *testing.T) {
	if os.Getenv(shellModelEnv) == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "child SIGTTOU ignored=%t\n", signal.Ignored(syscall.SIGTTOU))
}

func TestProductionShellModelHelper(t *testing.T) {
	job := os.Getenv(shellModelEnv)
	if job == "" {
		return
	}
	index := 0
	for index < len(os.Args) && os.Args[index] != argumentDelimiter {
		index++
	}
	args := os.Args[index+1:]
	process := exec.Command(args[0], args[1:]...) //nolint:gosec // Test-owned offline binary and literal argv.
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if job == jobForeground {
		process.SysProcAttr.Foreground = true
	} else if job == jobNoncontrolling {
		process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		_ = process.Process.Release()
	}()
	if err := os.WriteFile(os.Getenv("AZCTX_TEST_SHELL_JOB_PID"), []byte(strconv.Itoa(process.Process.Pid)), 0o600); err != nil { //nolint:gosec // Test-owned child cleanup record.
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(process.Process.Pid, &status, syscall.WUNTRACED|syscall.WNOHANG, nil)
		if err != nil {
			t.Fatal(err)
		}
		if pid == 0 {
			continue
		}
		if status.Stopped() {
			t.Fatalf("shell job stopped by %s", status.StopSignal())
		}
		group, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
		want := unix.Getpgrp()
		if job == jobForeground {
			want = process.Process.Pid
		}
		fmt.Printf("job exit=%d restored=%t\n", status.ExitStatus(), err == nil && group == want)
		return
	}
	t.Fatal("shell job did not finish")
}

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
			screen := recordPTY(master)
			interruptShellChild(ctx, t, master, screen, started, ready)
			err := process.Wait()
			_ = master.Close()
			output := screen.output()
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
