//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

// O_NOFOLLOW rejects a substituted leaf symlink; O_NONBLOCK avoids waiting on
// a substituted FIFO. Only a verified regular file is subsequently locked.
func openLock(path string) (*os.File, error) {
	const flags = syscall.O_CREAT | syscall.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_NOCTTY | syscall.O_CLOEXEC
	var fd int
	var err error
	for {
		fd, err = syscall.Open(path, flags, 0o600)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		return nil, &os.PathError{Op: openOp, Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func lock(file *os.File) (func(), bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }, true, nil
}
