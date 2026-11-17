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
	command := exec.Command(os.Args[3]) //nolint:gosec // Already resolved executable supplied by the parent, never shell code.
	command.Args = os.Args[4:]
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	for inherited := 3; inherited < fd; inherited++ {
		command.ExtraFiles = append(command.ExtraFiles, os.NewFile(uintptr(inherited), "child-extra"))
	}
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, forwardedSignals()...)
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
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return superviseWait(command, signals, done)
}

func superviseWait(command *exec.Cmd, signals <-chan os.Signal, done <-chan error) int {
	var timeout <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	interrupted, count := false, 0
	for {
		select {
		case err := <-done:
			if err != nil && command.ProcessState == nil {
				return 1
			}
			for len(signals) > 0 {
				interrupted = (<-signals == os.Interrupt) || interrupted
			}
			if interrupted {
				return 130
			}
			return ExitCode(command.ProcessState)
		case value := <-signals:
			// The child received the same group signal. Never forward it again.
			interrupted = interrupted || value == os.Interrupt
			count++
			if count > 1 {
				_ = command.Process.Kill()
			} else {
				timer = time.NewTimer(2 * time.Second)
				timeout = timer.C
			}
		case <-timeout:
			_ = command.Process.Kill()
			timeout = nil
		}
	}
}
