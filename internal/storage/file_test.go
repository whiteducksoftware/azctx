package storage_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const windowsOS = "windows"

func TestAtomicPrivateReplacement(t *testing.T) {
	base := testutil.WorkDir(t)
	azure := filepath.Join(base, "azure")
	name, err := storage.Path(base, azure, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Write(name, azure, []byte(`{"previous":"a much longer old value"}`)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := storage.Write(name, azure, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "{}" {
		t.Fatalf("replacement=%s error=%v", data, err)
	}
	if runtime.GOOS != windowsOS {
		for path, mode := range map[string]os.FileMode{filepath.Dir(name): 0o700, name: 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("private permissions: path=%s info=%v error=%v", path, info, err)
			}
		}
	}
	entries, err := os.ReadDir(filepath.Dir(name))
	if err != nil || len(entries) != 1 {
		t.Fatalf("atomic staging file leaked: %v, %v", entries, err)
	}
}

func TestProcessLockIsExclusiveAndReusable(t *testing.T) {
	base := testutil.WorkDir(t)
	azure := filepath.Join(base, "azure")
	name, err := storage.Path(base, azure, "update.json.lock")
	if err != nil {
		t.Fatal(err)
	}
	release, acquired, err := storage.Lock(name, azure)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v error=%v", acquired, err)
	}
	before, err := os.Stat(name)
	if err != nil {
		release()
		t.Fatal(err)
	}
	if runtime.GOOS != windowsOS && before.Mode().Perm() != 0o600 {
		release()
		t.Fatalf("lock is not private: %v", before.Mode())
	}
	// Pin the lazily loaded Windows file ID before releasing the first handle.
	_ = os.SameFile(before, before)
	secondRelease, acquired, err := storage.Lock(name, azure)
	if err != nil || acquired {
		if acquired {
			secondRelease()
		}
		release()
		t.Fatalf("concurrent lock: acquired=%v error=%v", acquired, err)
	}
	release()
	release, acquired, err = storage.Lock(name, azure)
	if err != nil || !acquired {
		t.Fatalf("reused lock: acquired=%v error=%v", acquired, err)
	}
	release()
	after, err := os.Stat(name)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("retained lock identity changed: before=%v after=%v error=%v", before, after, err)
	}
}

func TestRemovePrivateRegularFileAndMissingDestination(t *testing.T) {
	dir := testutil.WorkDir(t)
	path, azure := filepath.Join(dir, "state.json"), filepath.Join(dir, "azure")
	if err := storage.Write(path, azure, []byte("synthetic state")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := storage.Remove(path, azure); err != nil {
			t.Fatal(err)
		}
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("removed file or staging left behind: %v error=%v", entries, err)
	}
}

func TestEnvironmentCannotRedirectStorageIntoAzure(t *testing.T) {
	base := testutil.WorkDir(t)
	t.Setenv("AZURE_CONFIG_DIR", base)
	if _, err := storage.Path(base, filepath.Join(base, "other-azure"), "state.json"); err == nil {
		t.Fatal("Azure environment path must remain protected with an explicit injected config directory")
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatalf("path resolution wrote data: %v, %v", entries, err)
	}
}

func TestDirectWritesCannotCreateAnythingInAzureRoots(t *testing.T) {
	base := testutil.WorkDir(t)
	explicit := filepath.Join(base, "explicit-azure")
	environment := filepath.Join(base, "environment-azure")
	t.Setenv("AZURE_CONFIG_DIR", environment)
	for _, root := range []string{explicit, environment} {
		path := filepath.Join(root, "nested", "state.json")
		if err := storage.Write(path, explicit, []byte("synthetic")); err == nil {
			t.Fatalf("direct write accepted protected destination: %s", path)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected writes created Azure files or directories: %v, %v", entries, err)
	}
}
