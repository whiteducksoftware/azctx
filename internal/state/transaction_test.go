package state_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	firstTarget  = "22222222-2222-2222-2222-222222222222"
	secondTarget = "33333333-3333-3333-3333-333333333333"
)

func isolatedState(t *testing.T) (azure, path string) {
	t.Helper()
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", filepath.Join(dir, "azctx"))
	return filepath.Join(dir, "azure"), filepath.Join(dir, "azctx", "state.json")
}

func TestRollbackNeverErasesAnotherCommittedUpdate(t *testing.T) {
	azure, path := isolatedState(t)
	if err := state.Save(azure, subscriptionID); err != nil {
		t.Fatal(err)
	}
	failed, err := state.RecordSelection(azure, subscriptionID, firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	// A writer that read the failed switch's bytes commits before the rollback.
	if _, err := state.RecordSelection(azure, firstTarget, secondTarget); err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := failed(); !errors.Is(err, state.ErrChanged) {
		t.Fatalf("rollback over a newer update returned %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(committed) {
		t.Fatalf("rollback erased another committed update: %q, %v", after, err)
	}
}

func TestRollbackOfNewStateKeepsAnotherCommittedUpdate(t *testing.T) {
	azure, path := isolatedState(t)
	failed, err := state.RecordSelection(azure, "", firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"previousSubscriptionId":"`+subscriptionID+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := failed(); !errors.Is(err, state.ErrChanged) {
		t.Fatalf("rollback over a newer update returned %v", err)
	}
	if got, err := state.Load(azure); err != nil || got != subscriptionID {
		t.Fatalf("rollback removed another committed update: %q, %v", got, err)
	}
}

func TestReleasedTransactionCannotWrite(t *testing.T) {
	azure, path := isolatedState(t)
	tx, err := state.Begin(t.Context(), azure)
	if err != nil {
		t.Fatal(err)
	}
	tx.Release()
	tx.Release()
	if _, err := tx.RecordSelection(subscriptionID, firstTarget); err == nil {
		t.Fatal("released transaction wrote state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("released transaction created state: %v", err)
	}
	lock, err := os.Stat(filepath.Join(filepath.Dir(path), "state.lock"))
	if err != nil || !lock.Mode().IsRegular() {
		t.Fatalf("retained lock file missing after release: %v, %v", lock, err)
	}
}

func TestRolledBackTransactionRestoresExactBytesUnderLock(t *testing.T) {
	azure, path := isolatedState(t)
	const original = "{ \"previousSubscriptionId\": \"" + subscriptionID + "\" }\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	tx, err := state.Begin(t.Context(), azure)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Release()
	rollback, err := tx.RecordSelection(firstTarget, secondTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != original {
		t.Fatalf("rollback changed exact state bytes: %q, %v", after, err)
	}
}

func TestConcurrentSelectionsKeepEveryUpdate(t *testing.T) {
	azure, path := isolatedState(t)
	const writers = 24
	var group sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		group.Go(func() {
			_, err := state.RecordSelection(azure, subscriptionID, fmt.Sprintf("%08x-0000-0000-0000-000000000000", i+1))
			errs <- err
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Recent []string `json:"recentSubscriptionIds"`
	}
	if err := json.Unmarshal(data, &value); err != nil || len(value.Recent) != writers {
		t.Fatalf("concurrent switches lost recent IDs: %d of %d, %v", len(value.Recent), writers, err)
	}
}
