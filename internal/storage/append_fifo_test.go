//go:build !windows

package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAppendDoesNotBlockOnFIFOSwappedBeforeOpen(t *testing.T) {
	dir, path, _ := raceFiles(t)
	moved := filepath.Join(dir, "moved")
	withAppendSeams(t, func() {
		if err := os.Rename(path, moved); err != nil {
			t.Error(err)
			return
		}
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Error(err)
		}
	}, nil, nil)
	done := make(chan error, 1)
	go func() { done <- Append(path, "", []byte("INJECTED=bad\n")) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO destination accepted")
		}
	case <-time.After(5 * time.Second):
		// Unblock the stuck open so the test process can exit.
		if reader, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
			defer reader.Close()
		}
		t.Fatal("append blocked opening a swapped FIFO")
	}
	requireUnchanged(t, moved)
}
