package storage

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lock(file *os.File) (func(), bool, error) {
	handle := windows.Handle(file.Fd())
	overlapped := &windows.Overlapped{}
	err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { _ = windows.UnlockFileEx(handle, 0, 1, 0, overlapped) }, true, nil
}
