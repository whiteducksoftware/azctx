//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

func requireSingleLink(_ *os.File, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return errors.New("output destination must not have hard links")
	}
	return nil
}
