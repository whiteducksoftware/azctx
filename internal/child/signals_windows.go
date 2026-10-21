package child

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureProcess(command *exec.Cmd) {
	// Give the child a console process group so Ctrl+Break can target it rather
	// than interrupting azctx again. Redirected standard handles stay attached.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	command.Cancel = func() error {
		if err := forwardSignal(command.Process, os.Interrupt); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
}

func forwardedSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func forwardSignal(process *os.Process, _ os.Signal) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid)) //nolint:gosec // Windows process IDs are native unsigned 32-bit values.
}
