package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	privateWrite    = "write"
	privateRemove   = "remove"
	privateLock     = "lock"
	environmentRoot = "environment"
)

func storageSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		// ERROR_PRIVILEGE_NOT_HELD: Windows requires Developer Mode or symlink privilege.
		if runtime.GOOS == windowsOS && errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func mutatePrivateFile(operation, path, azure string) error {
	switch operation {
	case privateWrite:
		return storage.Write(path, azure, []byte("replacement"))
	case privateRemove:
		return storage.Remove(path, azure)
	default:
		release, _, err := storage.Lock(path, azure)
		if release != nil {
			release()
		}
		return err
	}
}

func requireStorageDirectoryMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || (runtime.GOOS != windowsOS && info.Mode().Perm() != mode) {
		t.Errorf("directory type or permissions changed: path=%s mode=%v want=%v", path, info.Mode(), mode)
	}
}

func TestPrivateMutationsRejectOutwardSymlinkInsideAzure(t *testing.T) {
	for _, root := range []string{"explicit", environmentRoot} {
		for _, operation := range []string{privateWrite, privateRemove, privateLock} {
			t.Run(root+"/"+operation, func(t *testing.T) {
				dir := testutil.WorkDir(t)
				azure := filepath.Join(dir, "azure")
				protected := azure
				if root == environmentRoot {
					t.Setenv("AZURE_CONFIG_DIR", azure)
					protected = filepath.Join(dir, "other-azure")
				}
				if err := os.Mkdir(azure, 0o755); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(dir, "external")
				if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(azure, "state.json")
				storageSymlink(t, target, path)
				if err := mutatePrivateFile(operation, path, protected); err == nil {
					t.Error("accepted a directory entry inside Azure configuration")
				}
				if link, err := os.Readlink(path); err != nil || link != target {
					t.Errorf("protected symlink changed: target=%q error=%v", link, err)
				}
				if data, err := os.ReadFile(target); err != nil || string(data) != "unchanged" {
					t.Errorf("external target changed: %q error=%v", data, err)
				}
				if entries, err := os.ReadDir(azure); err != nil || len(entries) != 1 {
					t.Errorf("protected entries changed: %v error=%v", entries, err)
				}
				requireStorageDirectoryMode(t, azure, 0o755)
			})
		}
	}
}

func TestPrivateMutationsRejectExternalLeafSymlinks(t *testing.T) {
	for _, operation := range []string{privateWrite, privateRemove, privateLock} {
		t.Run(operation, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			target := filepath.Join(dir, "external")
			if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "private")
			storageSymlink(t, target, path)
			if err := mutatePrivateFile(operation, path, filepath.Join(dir, "azure")); err == nil {
				t.Error("accepted a private-file leaf symlink")
			}
			if link, err := os.Readlink(path); err != nil || link != target {
				t.Errorf("leaf symlink changed: target=%q error=%v", link, err)
			}
			if data, err := os.ReadFile(target); err != nil || string(data) != "unchanged" {
				t.Errorf("external target changed: %q error=%v", data, err)
			}
		})
	}
}

func TestLockRejectsDanglingSymlinkWithoutCreatingAzureTarget(t *testing.T) {
	for _, root := range []string{"explicit", environmentRoot} {
		t.Run(root, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			azure, private := filepath.Join(dir, "azure"), filepath.Join(dir, "private")
			for _, name := range []string{azure, private} {
				if err := os.Mkdir(name, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			protected := azure
			if root == environmentRoot {
				t.Setenv("AZURE_CONFIG_DIR", azure)
				protected = filepath.Join(dir, "other-azure")
			}
			target := filepath.Join(azure, "lock-target")
			path := filepath.Join(private, "update.json.lock")
			storageSymlink(t, target, path)
			release, acquired, err := storage.Lock(path, protected)
			if release != nil {
				release()
			}
			if err == nil || acquired || release != nil {
				t.Errorf("dangling lock symlink accepted: acquired=%t release=%t error=%v", acquired, release != nil, err)
			}
			if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("protected target created: %v", err)
			}
			if link, err := os.Readlink(path); err != nil || link != target {
				t.Errorf("lock symlink changed: target=%q error=%v", link, err)
			}
			if entries, err := os.ReadDir(azure); err != nil || len(entries) != 0 {
				t.Errorf("protected directory changed: %v error=%v", entries, err)
			}
			for _, name := range []string{azure, private} {
				requireStorageDirectoryMode(t, name, 0o755)
			}
		})
	}
}

func TestStoragePathsRejectOutwardSymlinkEntryThroughAzureParent(t *testing.T) {
	dir := testutil.WorkDir(t)
	azure := filepath.Join(dir, "azure")
	private := filepath.Join(azure, "azctx")
	if err := os.MkdirAll(private, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "external")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(private, "state.json")
	storageSymlink(t, target, path)
	alias := filepath.Join(dir, "alias")
	storageSymlink(t, azure, alias)
	if _, err := storage.Path(alias, azure, "state.json"); err == nil {
		t.Error("Path accepted an outward symlink entry through a protected parent")
	}
	t.Setenv("AZCTX_CONFIG_DIR", filepath.Join(alias, "azctx"))
	if _, err := storage.ConfigPath(azure, "state.json"); err == nil {
		t.Error("ConfigPath accepted an outward symlink entry through a protected parent")
	}
	if link, err := os.Readlink(path); err != nil || link != target {
		t.Errorf("path validation changed symlink: target=%q error=%v", link, err)
	}
}

func TestPrivateMutationsRejectDirectoryLeaves(t *testing.T) {
	for _, operation := range []string{privateWrite, privateRemove, privateLock} {
		t.Run(operation, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			path := filepath.Join(dir, "private")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := mutatePrivateFile(operation, path, filepath.Join(dir, "azure")); err == nil {
				t.Error("accepted a directory as a private file")
			}
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				t.Errorf("directory leaf changed: %v error=%v", info, err)
			}
		})
	}
}

func TestPrivateRegularFilesThroughSafeSymlinkParent(t *testing.T) {
	dir := testutil.WorkDir(t)
	azure := filepath.Join(dir, "azure")
	private := filepath.Join(dir, "azure-sibling")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	storageSymlink(t, private, alias)
	path, lockPath := filepath.Join(alias, "state.json"), filepath.Join(alias, "state.lock")
	for _, data := range []string{"longer old state", "new state"} {
		if err := storage.Write(path, azure, []byte(data)); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Fatalf("regular replacement=%q error=%v", got, err)
		}
	}
	release, acquired, err := storage.Lock(lockPath, azure)
	if err != nil || !acquired {
		t.Fatalf("regular lock through parent: acquired=%t error=%v", acquired, err)
	}
	release()
	for range 2 {
		if err := storage.Remove(path, azure); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("regular file not removed: %v", err)
	}
	if entries, err := os.ReadDir(private); err != nil || len(entries) != 1 || entries[0].Name() != "state.lock" {
		t.Fatalf("staging leaked or retained lock lost: %v error=%v", entries, err)
	}
	if _, err := os.Stat(azure); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("regular storage created Azure directory: %v", err)
	}
}
