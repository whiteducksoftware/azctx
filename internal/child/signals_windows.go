package child

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureProcess(command *exec.Cmd) (processHooks, error) {
	// Give the child a console process group so Ctrl+Break can target it rather
	// than interrupting azctx again. Redirected standard handles stay attached.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	return processHooks{started: func() {}, finish: func() error { return nil }, close: func() {}}, nil
}

func forwardedSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func forwardSignal(process *os.Process, _ os.Signal) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid)) //nolint:gosec // Windows process IDs are native unsigned 32-bit values.
}

func killProcess(process *os.Process) error { return process.Kill() }
