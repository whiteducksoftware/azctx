package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func heldLock(t *testing.T) (path, azure string, release func()) {
	t.Helper()
	dir := testutil.WorkDir(t)
	path, azure = filepath.Join(dir, "azctx", "state.lock"), filepath.Join(dir, "azure")
	release, acquired, err := storage.Lock(path, azure)
	if err != nil || !acquired {
		t.Fatalf("hold lock: %t, %v", acquired, err)
	}
	return path, azure, release
}

func TestWaitLockRetriesUntilTheHolderReleases(t *testing.T) {
	path, azure, release := heldLock(t)
	busy, proceed := make(chan struct{}), make(chan struct{})
	acquired := make(chan func(), 1)
	go func() {
		unlock, err := storage.WaitLock(context.Background(), path, azure, func(context.Context) error {
			busy <- struct{}{}
			<-proceed
			return nil
		})
		if err != nil {
			t.Error(err)
		}
		acquired <- unlock
	}()
	<-busy
	select {
	case <-acquired:
		t.Fatal("waiter acquired a held lock")
	default:
	}
	release()
	close(proceed)
	unlock := <-acquired
	if _, again, err := storage.Lock(path, azure); again || err != nil {
		t.Fatalf("waiter does not hold the retained lock: %t, %v", again, err)
	}
	unlock()
	if next, again, err := storage.Lock(path, azure); !again || err != nil {
		t.Fatalf("waiter did not release the lock: %t, %v", again, err)
	} else {
		next()
	}
}

func TestWaitLockStopsWhenCanceled(t *testing.T) {
	path, azure, release := heldLock(t)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	waits := 0
	_, err := storage.WaitLock(ctx, path, azure, func(ctx context.Context) error {
		waits++
		cancel()
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || waits != 1 {
		t.Fatalf("canceled wait returned %v after %d waits", err, waits)
	}
	if _, err := storage.WaitLock(ctx, path, azure, func(context.Context) error {
		t.Fatal("canceled context attempted to wait")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled wait returned %v", err)
	}
}
