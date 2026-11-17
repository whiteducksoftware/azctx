//go:build !windows

package storage_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func statT(t *testing.T, path string) (os.FileInfo, *syscall.Stat_t) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("no Unix stat for %s", path)
	}
	return info, stat
}

func TestAppendPreservesInodeOwnerAndMode(t *testing.T) {
	dir := testutil.WorkDir(t)
	path := filepath.Join(dir, "environment")
	if err := os.WriteFile(path, []byte("EXISTING=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// WriteFile honors umask; set the runner-like mode explicitly.
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	beforeInfo, before := statT(t, path)
	if err := storage.Append(path, "", []byte("NEXT=value\n")); err != nil {
		t.Fatal(err)
	}
	afterInfo, after := statT(t, path)
	if after.Ino != before.Ino || after.Dev != before.Dev || after.Uid != before.Uid || after.Gid != before.Gid || afterInfo.Mode() != beforeInfo.Mode() || after.Nlink != 1 {
		t.Fatalf("identity changed: inode %d->%d uid %d->%d gid %d->%d mode %v->%v nlink %d",
			before.Ino, after.Ino, before.Uid, after.Uid, before.Gid, after.Gid, beforeInfo.Mode(), afterInfo.Mode(), after.Nlink)
	}
}

// Container jobs append to a runner-owned file in a runner-owned directory the
// step user cannot create files in. Only the file itself is writable.
func TestAppendNeedsOnlyFileWritePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions; covered by TestAppendPreservesForeignOwnerAsRoot")
	}
	dir := testutil.WorkDir(t)
	runner := filepath.Join(dir, "file_commands")
	if err := os.Mkdir(runner, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runner, "set_env")
	if err := os.WriteFile(path, []byte("EXISTING=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runner, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runner, 0o700) })
	if err := storage.Append(path, "", []byte("NEXT=value\n")); err != nil {
		t.Fatalf("append needed directory write access: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "EXISTING=value\nNEXT=value\n" {
		t.Fatalf("contents=%q, %v", data, err)
	}
	if info, _ := statT(t, path); info.Mode().Perm() != 0o666 {
		t.Fatalf("mode changed to %v", info.Mode())
	}
}

// The live container case (root step appending to a host-user file) needs
// root to create a differently owned file. Unprivileged runs skip it.
func TestAppendPreservesForeignOwnerAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to chown a file to another owner; NOT verified in this run")
	}
	dir := testutil.WorkDir(t)
	path := filepath.Join(dir, "environment")
	if err := os.WriteFile(path, []byte("EXISTING=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const foreign = 65534
	if err := os.Chown(path, foreign, foreign); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, before := statT(t, path)
	if err := storage.Append(path, "", []byte("NEXT=value\n")); err != nil {
		t.Fatal(err)
	}
	info, after := statT(t, path)
	if after.Uid != foreign || after.Gid != foreign || after.Ino != before.Ino || info.Mode().Perm() != 0o644 {
		t.Fatalf("foreign file changed: uid=%d gid=%d inode %d->%d mode=%v", after.Uid, after.Gid, before.Ino, after.Ino, info.Mode())
	}
}
