package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	lockedPrevious = "11111111-1111-1111-1111-111111111111"
	lockedFirst    = "22222222-2222-2222-2222-222222222222"
	lockedSecond   = "33333333-3333-3333-3333-333333333333"
	lockedOther    = "44444444-4444-4444-4444-444444444444"
)

// waitingLock replaces the retry delay with explicit steps: busy reports each
// failed attempt and the waiter retries only when the test sends on proceed.
func waitingLock(t *testing.T) (busy chan struct{}, proceed chan struct{}) {
	t.Helper()
	busy, proceed = make(chan struct{}), make(chan struct{})
	original := lockWait
	lockWait = func(ctx context.Context) error {
		busy <- struct{}{}
		select {
		case <-proceed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.Cleanup(func() { lockWait = original })
	return busy, proceed
}

func lockedState(t *testing.T) (azure, path string, before []byte) {
	t.Helper()
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", filepath.Join(dir, "azctx"))
	azure, path = filepath.Join(dir, "azure"), filepath.Join(dir, "azctx", "state.json")
	before = []byte("{ \"previousSubscriptionId\": \"" + lockedPrevious + "\", \"recentSubscriptionIds\": [\"" + lockedOther + "\"] }\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	return azure, path, before
}

func TestTransactionsSerializeAroundAzureSwitch(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failed     bool
		wantRecent []string
	}{
		{"first switch fails", true, []string{lockedSecond, lockedOther}},
		{"first switch commits", false, []string{lockedSecond, lockedFirst, lockedOther}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			azure, path, before := lockedState(t)
			busy, proceed := waitingLock(t)
			first, err := Begin(t.Context(), azure)
			if err != nil {
				t.Fatal(err)
			}
			rollback, err := first.RecordSelection(lockedPrevious, lockedFirst)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				second, err := Begin(context.Background(), azure)
				if err != nil {
					done <- err
					return
				}
				defer second.Release()
				_, err = second.RecordSelection(lockedFirst, lockedSecond)
				done <- err
			}()
			// The second switch is now provably waiting while the first runs az.
			<-busy
			select {
			case err := <-done:
				t.Fatalf("second transaction bypassed the retained lock: %v", err)
			default:
			}
			if tc.failed {
				if err := rollback(); err != nil {
					t.Fatal(err)
				}
				if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
					t.Fatalf("failed switch did not restore exact bytes: %q, %v", after, err)
				}
			}
			first.Release()
			close(proceed)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			value, err := read(azure)
			if err != nil || value.PreviousSubscriptionID != lockedFirst || !slices.Equal(value.RecentSubscriptionIDs, tc.wantRecent) {
				t.Fatalf("serialized state = %+v, %v", value, err)
			}
		})
	}
}

func TestCanceledLockWaitLeavesStateUnchanged(t *testing.T) {
	azure, path, before := lockedState(t)
	busy, _ := waitingLock(t)
	holder, err := Begin(t.Context(), azure)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		tx, err := Begin(ctx, azure)
		if err == nil {
			tx.Release()
		}
		done <- err
	}()
	<-busy
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lock wait returned %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("canceled lock wait changed state: %q, %v", after, err)
	}
}

func TestCanceledContextNeverAttemptsLock(t *testing.T) {
	azure, path, _ := lockedState(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Begin(ctx, azure); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled begin returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "state.lock")); !os.IsNotExist(err) {
		t.Fatalf("canceled begin created a lock file: %v", err)
	}
}
