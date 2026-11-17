//go:build !windows

package child

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func init() {
	if len(os.Args) >= 5 && os.Args[1] == supervisorArg && os.Getenv(supervisorEnv) == strconv.Itoa(os.Getppid()) {
		if err := os.Unsetenv(supervisorEnv); err != nil {
			os.Exit(1)
		}
		os.Exit(supervise())
	}
}

func supervise() int {
	fd, err := strconv.Atoi(os.Args[2])
	if err != nil || fd < 3 {
		return 1
	}
	startError := os.NewFile(uintptr(fd), "child-start-error")
	unix.CloseOnExec(fd)
	defer startError.Close()
	jobControl := os.Getenv(supervisorJobEnv) == "true"
	if err := os.Unsetenv(supervisorJobEnv); err != nil {
		return 1
	}
	command := exec.Command(os.Args[3]) //nolint:gosec // Already resolved executable supplied by the parent, never shell code.
	command.Args = os.Args[4:]
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	for inherited := 3; inherited < fd; inherited++ {
		command.ExtraFiles = append(command.ExtraFiles, os.NewFile(uintptr(inherited), "child-extra"))
	}
	signals := make(chan os.Signal, 8)
	changes := make(chan os.Signal, 1)
	signal.Notify(changes, syscall.SIGCHLD)
	defer signal.Stop(changes)
	// Stay runnable when a terminal stop signal stops the target. Only a
	// confirmed wait4 stop propagates suspension to the shell-visible parent.
	signal.Notify(signals, append(forwardedSignals(), syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU, syscall.SIGCONT)...)
	defer signal.Stop(signals)
	if err := command.Start(); err != nil {
		var pathError *os.PathError
		var errno syscall.Errno
		failure := startupFailure{Message: err.Error()}
		if errors.As(err, &pathError) && errors.As(err, &errno) {
			failure.Op, failure.Path, failure.Errno = pathError.Op, pathError.Path, uint64(errno)
		}
		_ = json.NewEncoder(startError).Encode(failure)
		return 1
	}
	_ = startError.Close()
	defer func() { _ = command.Process.Release() }()
	states := make(chan childState)
	go waitChild(command.Process.Pid, states, changes)
	return superviseWait(command, signals, states, jobControl)
}

type childState struct {
	status unix.WaitStatus
	err    error
}

func waitChild(pid int, states chan<- childState, changes <-chan os.Signal) {
	// This is the target's only wait/reap consumer. Cmd.Wait cannot observe
	// stops. All target streams are *os.File, with no exec-owned copy goroutines
	// or context watcher to join; Release closes its process handle after reaping.
	// Drain nonblocking before waiting for SIGCHLD, whose notifications coalesce.
	for {
		var status unix.WaitStatus
		waited, err := unix.Wait4(pid, &status, unix.WUNTRACED|unix.WNOHANG, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err == nil && waited == 0 {
			<-changes
			continue
		}
		states <- childState{status, err}
		if err != nil || status.Exited() || status.Signaled() {
			return
		}
	}
}

func (s childState) exitCode(signals <-chan os.Signal, interrupted bool) int {
	if s.err != nil {
		return 1
	}
	for len(signals) > 0 {
		interrupted = (<-signals == os.Interrupt) || interrupted
	}
	if interrupted {
		return 130
	}
	if s.status.Signaled() {
		return 128 + int(s.status.Signal())
	}
	return s.status.ExitStatus()
}

func childStopped(canceling, jobControl bool) bool {
	if canceling {
		// A stopped target cannot handle pending cancellation.
		_ = unix.Kill(-unix.Getpgrp(), unix.SIGCONT)
		return false
	}
	if jobControl {
		_ = unix.Kill(os.Getppid(), unix.SIGTSTP)
	}
	return true
}

func superviseWait(command *exec.Cmd, signals <-chan os.Signal, states <-chan childState, jobControl bool) int {
	var timeout <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	interrupted, stopped, count := false, false, 0
	for {
		select {
		case state := <-states:
			if state.err != nil || state.status.Exited() || state.status.Signaled() {
				return state.exitCode(signals, interrupted)
			}
			// Only stops and exits are requested, never continued states.
			// Do not use WaitStatus.Stopped: on BSD its continued-state encoding
			// overlaps SIGSTOP, which must also remain cancellable.
			stopped = childStopped(count > 0, jobControl)
		case value := <-signals:
			switch value {
			case syscall.SIGCONT:
				stopped = false
				continue
			case syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU:
				continue
			}
			// The child received the same group signal. Never forward it again.
			interrupted = interrupted || value == os.Interrupt
			count++
			if count > 1 {
				_ = command.Process.Kill()
			} else {
				timer = time.NewTimer(2 * time.Second)
				timeout = timer.C
			}
			if stopped {
				_ = unix.Kill(-unix.Getpgrp(), unix.SIGCONT)
				stopped = false
			}
		case <-timeout:
			_ = command.Process.Kill()
			timeout = nil
		}
	}
}
