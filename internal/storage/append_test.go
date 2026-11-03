package storage_test

import (
	"io"
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

func TestAppendKeepsFileIdentityModeAndOpenReaders(t *testing.T) {
	dir := testutil.WorkDir(t)
	path := filepath.Join(dir, "environment")
	if err := os.WriteFile(path, []byte("EXISTING=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Append(path, filepath.Join(dir, ".azure"), []byte("NEXT=value\n")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode() != before.Mode() {
		t.Fatalf("destination replaced: before=%v after=%v same=%v", before.Mode(), after.Mode(), os.SameFile(before, after))
	}
	// A handle opened before the append must observe the new data, as the runner's would.
	seen, err := io.ReadAll(reader)
	if err != nil || string(seen) != "EXISTING=value\nNEXT=value\n" {
		t.Fatalf("pre-opened handle saw %q, %v", seen, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("unexpected staging files: %v, %v", entries, err)
	}
}

func TestAppendSeparatorOnlyWhenNeeded(t *testing.T) {
	for _, existing := range []string{"", "A=1\n", "A=1"} {
		dir := testutil.WorkDir(t)
		path := filepath.Join(dir, "environment")
		if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := storage.Append(path, "", []byte("B=2\n")); err != nil {
			t.Fatal(err)
		}
		want := strings.TrimSuffix(existing, "\n")
		if want != "" {
			want += "\n"
		}
		want += "B=2\n"
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("existing %q: got %q, %v", existing, data, err)
		}
	}
}

func TestAppendRefusesUnsafeDestinationsWithoutChanges(t *testing.T) {
	for _, scenario := range []string{"symlink", "hardlink", "readonly", "directory", "azure", "azure symlink"} {
		t.Run(scenario, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			azure := filepath.Join(dir, ".azure")
			if err := os.Mkdir(azure, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(target, []byte("unchanged\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			protected := filepath.Join(azure, "config")
			if err := os.WriteFile(protected, []byte("unchanged\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := unsafeAppendDestination(t, scenario, dir, target, protected)
			before, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := storage.Append(path, azure, []byte("INJECTED=bad\n")); err == nil {
				t.Fatal("unsafe destination accepted")
			}
			for _, name := range []string{target, protected} {
				if data, err := os.ReadFile(name); err != nil || string(data) != "unchanged\n" {
					t.Fatalf("%s changed: %q, %v", name, data, err)
				}
			}
			if after, err := os.ReadDir(dir); err != nil || len(after) != len(before) {
				t.Fatalf("directory entries changed: before=%v after=%v error=%v", before, after, err)
			}
		})
	}
}

func unsafeAppendDestination(t *testing.T, scenario, dir, target, protected string) string {
	t.Helper()
	path := filepath.Join(dir, "environment")
	switch scenario {
	case "symlink":
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	case "hardlink":
		if err := os.Link(target, path); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
	case "readonly":
		if err := os.Chmod(target, 0o400); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(target, 0o600) })
		path = target
	case "directory":
		path = dir
	case "azure":
		path = protected
	case "azure symlink":
		if err := os.Symlink(protected, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	return path
}
