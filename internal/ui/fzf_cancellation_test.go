package ui_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestFZFCancelsRunningProcess(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("fzf")), "github.com/whiteducksoftware/azctx/internal/testutil/fakefzf")
	t.Setenv("PATH", dir)
	record := filepath.Join(dir, "record.json")
	t.Setenv("FAKEFZF_RECORD", record)
	t.Setenv("FAKEFZF_WAIT", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := ui.SelectFZF(ctx, []profile.Subscription{{Id: productionID}}, io.Discard)
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(record); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake fzf never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active cancellation=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fzf survived command cancellation")
	}
}
