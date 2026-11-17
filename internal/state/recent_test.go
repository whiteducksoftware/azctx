package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRecentSelectionAndRollback(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	const next = "22222222-2222-2222-2222-222222222222"
	azure := filepath.Join(dir, "azure")
	if recent, err := state.Recent(azure); err != nil || len(recent) != 0 {
		t.Fatalf("missing recent = %v, %v", recent, err)
	}
	rollback, err := state.RecordSelection(azure, subscriptionID, next)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := state.Load(azure); err != nil || got != subscriptionID {
		t.Fatalf("previous = %q, %v", got, err)
	}
	if got, err := state.Recent(azure); err != nil || !reflect.DeepEqual(got, []string{next}) {
		t.Fatalf("recent = %v, %v", got, err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("rollback should remove newly created state: %v", err)
	}
}

func TestRecentMostRecentFirst(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	const next = "22222222-2222-2222-2222-222222222222"
	azure := filepath.Join(dir, "azure")
	if _, err := state.RecordSelection(azure, "", next); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Load(azure); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recent-only state must not invent a previous subscription: %v", err)
	}
	if _, err := state.RecordSelection(azure, next, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordSelection(azure, subscriptionID, next); err != nil {
		t.Fatal(err)
	}
	if got, err := state.Recent(azure); err != nil || !reflect.DeepEqual(got, []string{next, subscriptionID}) {
		t.Fatalf("MRU = %v, %v", got, err)
	}
}
