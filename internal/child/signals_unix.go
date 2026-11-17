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
	supervisorArg    = "--azctx-internal-supervisor"
	supervisorEnv    = "AZCTX_INTERNAL_SUPERVISOR"
	supervisorJobEnv = "AZCTX_INTERNAL_JOB_CONTROL"
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
	terminal, err := foreground(command)
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
	command.Env = append(command.Env, supervisorJobEnv+"="+strconv.FormatBool(terminal != nil))
	hooks := processHooks{
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
			return errors.Join(startErr, readErr, terminal.restore())
		},
		close: func() {
			_ = readError.Close()
			_ = writeError.Close()
		},
	}
	if terminal != nil {
		hooks.signals = []os.Signal{syscall.SIGTSTP, syscall.SIGCONT}
		hooks.signal = func(value os.Signal, canceling bool) (bool, error) {
			return terminal.signal(command, value, canceling)
		}
	}
	return hooks, nil
}

type startupFailure struct {
	Op      string `json:"op"`
	Path    string `json:"path"`
	Errno   uint64 `json:"errno"`
	Message string `json:"message"`
}

type foregroundTerminal struct {
	fd, group int
	owned     bool
}

func foreground(command *exec.Cmd) (*foregroundTerminal, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	file, ok := command.Stdin.(*os.File)
	if !terminalHandoff || !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, nil
	}
	fd := int(file.Fd())
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if errors.Is(err, unix.ENOTTY) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read terminal foreground group: %w", err)
	}
	if group != unix.Getpgrp() {
		return nil, nil
	}
	command.Stdout = terminalOutput(command.Stdout)
	command.Stderr = terminalOutput(command.Stderr)
	command.SysProcAttr.Foreground, command.SysProcAttr.Ctty = true, fd
	return &foregroundTerminal{fd: fd, group: group, owned: true}, nil
}

func (t *foregroundTerminal) give(group int) error {
	return terminalAccess(func() error {
		return unix.IoctlSetPointerInt(t.fd, unix.TIOCSPGRP, group)
	})
}

func (t *foregroundTerminal) restore() error {
	if t == nil || !t.owned {
		return nil
	}
	if err := t.give(t.group); err != nil {
		return fmt.Errorf("restore terminal foreground group: %w", err)
	}
	t.owned = false
	return nil
}

func (t *foregroundTerminal) signal(command *exec.Cmd, value os.Signal, canceling bool) (bool, error) {
	switch value {
	case syscall.SIGTSTP:
		// The supervisor reports an actual stopped child, not just delivery of
		// SIGTSTP (which a child can catch or ignore). Cancellation must not stop
		// azctx while its kill deadline is running.
		if canceling {
			return true, nil
		}
		if err := t.restore(); err != nil {
			return true, err
		}
		// Stop the shell-visible job, including any shell pipeline peers.
		return true, unix.Kill(-t.group, unix.SIGSTOP)
	case syscall.SIGCONT:
		group, err := unix.IoctlGetInt(t.fd, unix.TIOCGPGRP)
		if err != nil {
			return true, fmt.Errorf("read resumed terminal foreground group: %w", err)
		}
		// fg gives azctx its terminal before SIGCONT. bg must not steal it.
		if group == t.group {
			if err := t.give(command.Process.Pid); err != nil {
				if errors.Is(unix.Kill(-command.Process.Pid, 0), unix.ESRCH) {
					return true, nil
				}
				return true, fmt.Errorf("resume terminal foreground group: %w", err)
			}
			t.owned = true
		}
		err = unix.Kill(-command.Process.Pid, unix.SIGCONT)
		if errors.Is(err, unix.ESRCH) {
			err = nil
		}
		return true, err
	default:
		return false, nil
	}
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
