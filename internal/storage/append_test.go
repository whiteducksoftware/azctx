package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestAppendPreservesDataAndParentWithoutAzureWrites(t *testing.T) {
	dir := testutil.WorkDir(t)
	path := filepath.Join(dir, "environment")
	if err := os.WriteFile(path, []byte("EXISTING=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := storage.Append(path, filepath.Join(dir, ".azure"), []byte("NEXT=value\n")); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "EXISTING=value\nNEXT=value\nNEXT=value\n" {
		t.Fatalf("append contents=%q error=%v", data, err)
	}
	after, err := os.Stat(dir)
	if err != nil || after.Mode() != before.Mode() {
		t.Fatalf("parent permissions changed: before=%v after=%v error=%v", before, after, err)
	}
}

func TestAppendRejectsLargeOrMissingDestination(t *testing.T) {
	dir := testutil.WorkDir(t)
	path := filepath.Join(dir, "environment")
	if err := storage.Append(path, "", []byte("must not create\n")); !os.IsNotExist(err) {
		t.Fatalf("missing output accepted: %v", err)
	}
	const limit = 8 << 20
	data := []byte(strings.Repeat(" ", limit+1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := storage.Append(path, "", []byte("must not append\n")); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized file accepted: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != limit+1 {
		t.Fatalf("oversized file changed: %v, %v", info, err)
	}
}
