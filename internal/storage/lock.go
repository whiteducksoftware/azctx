package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Lock takes a nonblocking process lock. The private lock file is intentionally
// retained: unlinking a locked file would let another process lock a new inode.
// The OS releases the lock even if the owning process crashes.
func Lock(path, azureDir string) (release func(), acquired bool, err error) {
	if _, err := privateFileInfo(path, azureDir); err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	info, err := privateFileInfo(path, azureDir)
	if err != nil {
		return nil, false, err
	}
	// Windows loads Lstat file IDs lazily by path; pin an existing file's ID.
	if info != nil {
		_ = os.SameFile(info, info)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	file, err := openLock(path)
	if err != nil {
		return nil, false, err
	}
	if err := verifyLockTarget(file, path, azureDir, info); err != nil {
		_ = file.Close()
		return nil, false, err
	}
	unlock, acquired, err := lock(file)
	if !acquired || err != nil {
		_ = file.Close()
		return nil, false, err
	}
	return func() {
		unlock()
		_ = file.Close()
	}, true, nil
}

func verifyLockTarget(file *os.File, path, azureDir string, before os.FileInfo) error {
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || (before != nil && !os.SameFile(before, opened)) {
		return errors.New("azctx lock destination changed before open")
	}
	current, err := privateFileInfo(path, azureDir)
	if err != nil {
		return err
	}
	if current == nil || !os.SameFile(opened, current) {
		return errors.New("azctx lock destination changed while preparing lock")
	}
	return nil
}

// WaitLock retries the nonblocking Lock until it is acquired, calling wait
// after each busy attempt. Cancellation stops waiting without taking the lock.
func WaitLock(ctx context.Context, path, azureDir string, wait func(context.Context) error) (release func(), err error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, acquired, err := Lock(path, azureDir)
		if err != nil {
			return nil, err
		}
		if acquired {
			return release, nil
		}
		if err := wait(ctx); err != nil {
			return nil, err
		}
	}
}
