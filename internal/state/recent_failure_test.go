package state_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRecentRollbackAndCorruption(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	const next = "22222222-2222-2222-2222-222222222222"
	azure := filepath.Join(dir, "azure")
	if _, err := state.RecordSelection(azure, subscriptionID, next); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := state.RecordSelection(azure, next, subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rollback failed to restore exact state bytes: %q, %v", after, err)
	}
	corrupt := []byte(`{"previousSubscriptionId":"` + subscriptionID + `","recentSubscriptionIds":["invalid"]}`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Recent(azure); err == nil {
		t.Fatal("malformed recent IDs were accepted")
	}
	if _, err := state.RecordSelection(azure, subscriptionID, next); err == nil {
		t.Fatal("switch overwrote corrupt state")
	}
	after, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(corrupt, after) {
		t.Fatalf("corrupt state was overwritten: %q, %v", after, err)
	}
}
