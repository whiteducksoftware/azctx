package storage

import (
	"context"
	"os"
	"path/filepath"
)

// Lock takes a nonblocking process lock. The private lock file is intentionally
// retained: unlinking a locked file would let another process lock a new inode.
// The OS releases the lock even if the owning process crashes.
func Lock(path, azureDir string) (release func(), acquired bool, err error) {
	if err := validate(path, azureDir); err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	if err := validate(path, azureDir); err != nil {
		return nil, false, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
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
