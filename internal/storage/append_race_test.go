package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	unchanged     = "unchanged\n"
	swapRegular   = "regular"
	changeRenamed = "renamed"
)

func withAppendSeams(t *testing.T, beforeOpen, afterOpen func(), write func(*os.File, []byte) (int, error)) {
	t.Helper()
	oldBefore, oldAfter, oldWrite := appendBeforeOpen, appendAfterOpen, appendWrite
	t.Cleanup(func() { appendBeforeOpen, appendAfterOpen, appendWrite = oldBefore, oldAfter, oldWrite })
	if beforeOpen != nil {
		appendBeforeOpen = beforeOpen
	}
	if afterOpen != nil {
		appendAfterOpen = afterOpen
	}
	if write != nil {
		appendWrite = write
	}
}

func raceFiles(t *testing.T) (dir, path, other string) {
	t.Helper()
	dir = testutil.WorkDir(t)
	path, other = filepath.Join(dir, "environment"), filepath.Join(dir, "other")
	for _, name := range []string{path, other} {
		if err := os.WriteFile(name, []byte(unchanged), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, path, other
}

func requireUnchanged(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(name); err != nil || string(data) != unchanged {
			t.Fatalf("%s changed: %q, %v", name, data, err)
		}
	}
}

func TestAppendRejectsDestinationSwappedBeforeOpen(t *testing.T) {
	for _, swap := range []string{swapRegular, "symlink"} {
		t.Run(swap, func(t *testing.T) {
			dir, path, other := raceFiles(t)
			moved := filepath.Join(dir, "moved")
			var swapErr error
			withAppendSeams(t, func() {
				if swapErr = os.Rename(path, moved); swapErr != nil {
					return
				}
				if swapRegular == swap {
					// A distinct single-link file, so only identity checks can reject it.
					swapErr = os.WriteFile(path, []byte(unchanged), 0o600)
					return
				}
				swapErr = os.Symlink(other, path)
			}, nil, nil)
			err := Append(path, "", []byte("INJECTED=bad\n"))
			if swap == "symlink" && swapErr != nil {
				t.Skipf("symlinks unavailable: %v", swapErr)
			}
			if swapErr != nil {
				t.Fatal(swapErr)
			}
			if err == nil {
				t.Fatal("swapped destination accepted")
			}
			requireUnchanged(t, other, moved)
			if swapRegular == swap {
				requireUnchanged(t, path)
			}
		})
	}
}

func TestAppendRejectsDestinationChangedAfterOpen(t *testing.T) {
	for _, change := range []string{changeRenamed, "linked"} {
		t.Run(change, func(t *testing.T) {
			dir, path, other := raceFiles(t)
			moved := filepath.Join(dir, "moved")
			var changeErr error
			withAppendSeams(t, nil, func() {
				if change == "linked" {
					changeErr = os.Link(path, filepath.Join(dir, "link"))
					return
				}
				if changeErr = os.Rename(path, moved); changeErr == nil {
					changeErr = os.Rename(other, path)
				}
			}, nil)
			err := Append(path, "", []byte("INJECTED=bad\n"))
			if changeErr != nil && runtime.GOOS == "windows" && change == changeRenamed {
				// The open handle denies delete sharing, so the swap itself is refused.
				if err != nil {
					t.Fatalf("append failed although the destination was pinned: %v", err)
				}
				if data, readErr := os.ReadFile(path); readErr != nil || string(data) != unchanged+"INJECTED=bad\n" {
					t.Fatalf("pinned destination=%q, %v", data, readErr)
				}
				return
			}
			if changeErr != nil {
				t.Skipf("change unavailable: %v", changeErr)
			}
			if err == nil {
				t.Fatal("changed destination accepted")
			}
			requireUnchanged(t, path)
			if change == changeRenamed {
				requireUnchanged(t, moved)
			}
		})
	}
}

func TestAppendReportsWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		written int
		err     error
		want    string
	}{
		{"error", 0, errors.New("disk full"), "disk full"},
		{"partial error", 3, errors.New("disk full"), "partial append: wrote 3 of 13 bytes"},
		{"short write", 3, nil, "partial append: wrote 3 of 13 bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path, _ := raceFiles(t)
			var payloads [][]byte
			withAppendSeams(t, nil, nil, func(_ *os.File, data []byte) (int, error) {
				payloads = append(payloads, append([]byte(nil), data...))
				return tc.written, tc.err
			})
			err := Append(path, "", []byte("NEXT=value12\n"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("cause not wrapped: %v", err)
			}
			if len(payloads) != 1 || string(payloads[0]) != "NEXT=value12\n" {
				t.Fatalf("payload not written once: %q", payloads)
			}
		})
	}
}
