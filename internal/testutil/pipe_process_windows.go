package testutil

import (
	"fmt"
	"math"

	"golang.org/x/sys/windows"
)

func watchPipeProcess(pid int) (wait, release func() error, err error) {
	if pid <= 0 || pid > math.MaxUint32 {
		return nil, nil, fmt.Errorf("invalid stderr descendant PID %d", pid)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, nil, err
	}
	wait = func() error {
		event, err := windows.WaitForSingleObject(handle, pipeFixtureTimeoutMillis)
		if err != nil {
			return err
		}
		if event != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("stderr descendant %d did not exit: wait event %d", pid, event)
		}
		return nil
	}
	return wait, func() error { return windows.CloseHandle(handle) }, nil
}
