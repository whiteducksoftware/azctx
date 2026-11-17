//go:build !windows

package child

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	supervisorArg = "--azctx-internal-supervisor"
	supervisorEnv = "AZCTX_INTERNAL_SUPERVISOR"
)

// A supervisor shares the child's group, but azctx does not. It observes group
// interrupts without forwarding them. This also distinguishes a caught Ctrl+C
// from an ordinary exit 1 without requiring nonportable siginfo handlers.
func configureProcess(command *exec.Cmd) (processHooks, error) {
	self, err := os.Executable()
	if err != nil {
		return processHooks{}, err
	}
	readError, writeError, err := os.Pipe()
	if err != nil {
		return processHooks{}, err
	}
	restore, err := foreground(command)
	if err != nil {
		_ = readError.Close()
		_ = writeError.Close()
		return processHooks{}, err
	}
	fd := 3 + len(command.ExtraFiles)
	command.ExtraFiles = append(command.ExtraFiles, writeError)
	command.Args = append([]string{self, supervisorArg, strconv.Itoa(fd), command.Path}, command.Args...)
	command.Path = self
	command.Env = append(command.Environ(), supervisorEnv+"="+strconv.Itoa(os.Getpid()))
	return processHooks{
		started: func() { _ = writeError.Close() },
		finish: func() error {
			data, readErr := io.ReadAll(readError)
			var startErr error
			if len(data) != 0 {
				command.ProcessState = nil
				var failure startupFailure
				if err := json.Unmarshal(data, &failure); err != nil {
					startErr = fmt.Errorf("decode child startup failure: %w", err)
				} else {
					if failure.Op == "" {
						startErr = errors.New(failure.Message)
					} else {
						startErr = &os.PathError{Op: failure.Op, Path: failure.Path, Err: syscall.Errno(failure.Errno)}
					}
				}
			}
			return errors.Join(startErr, readErr, restore())
		},
		close: func() {
			_ = readError.Close()
			_ = writeError.Close()
		},
	}, nil
}

type startupFailure struct {
	Op      string `json:"op"`
	Path    string `json:"path"`
	Errno   uint64 `json:"errno"`
	Message string `json:"message"`
}

func foreground(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	file, ok := command.Stdin.(*os.File)
	if !terminalHandoff || !ok || !term.IsTerminal(int(file.Fd())) {
		return func() error { return nil }, nil
	}
	fd := int(file.Fd())
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if errors.Is(err, unix.ENOTTY) {
		return func() error { return nil }, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read terminal foreground group: %w", err)
	}
	if group != unix.Getpgrp() {
		return func() error { return nil }, nil
	}
	command.Stdout = terminalOutput(command.Stdout)
	command.Stderr = terminalOutput(command.Stderr)
	command.SysProcAttr.Foreground, command.SysProcAttr.Ctty = true, fd
	return func() error {
		if err := terminalAccess(func() error {
			return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group)
		}); err != nil {
			return fmt.Errorf("restore terminal foreground group: %w", err)
		}
		return nil
	}, nil
}

func forwardedSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
}

func forwardSignal(process *os.Process, value os.Signal) error {
	sig, ok := value.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported signal %v", value)
	}
	return unix.Kill(-process.Pid, sig)
}

func killProcess(process *os.Process) error { return unix.Kill(-process.Pid, unix.SIGKILL) }
