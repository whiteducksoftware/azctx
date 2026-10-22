//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

// openAppend opens path for in-place appends without following a final
// symlink. O_NONBLOCK keeps a FIFO swapped in after Lstat from blocking the
// open; it is cleared before writing to the (verified) regular file.
func openAppend(path string) (*os.File, error) {
	const flags = syscall.O_RDWR | syscall.O_APPEND | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_NOCTTY | syscall.O_CLOEXEC
	var fd int
	var err error
	for {
		fd, err = syscall.Open(path, flags, 0)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, errors.New("output destination must not be a symlink")
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := syscall.SetNonblock(fd, false); err != nil {
		_ = syscall.Close(fd)
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func requireSingleLink(_ *os.File, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return errors.New("output destination must not have hard links")
	}
	return nil
}
