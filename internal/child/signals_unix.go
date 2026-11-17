//go:build !windows

package child

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
}

func forwardedSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
}

func forwardSignal(process *os.Process, value os.Signal) error {
	return process.Signal(value)
}
