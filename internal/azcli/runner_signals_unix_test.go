//go:build !windows

package azcli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestLoginRunnerCancellationInterruptsChild(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, "az"), "github.com/whiteducksoftware/azctx/internal/testutil/contextchild")
	ready := filepath.Join(dir, "ready")
	t.Setenv("PATH", dir)
	t.Setenv("AZCTX_TEST_CHILD_MODE", "trap")
	t.Setenv("AZCTX_TEST_CHILD_READY", ready)
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		result, err := runner.Run(ctx, []string{loginCommand}, azcli.Options{
			Terminal: &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr},
		})
		if result.ExitCode != 130 {
			err = errors.Join(err, errors.New("child exit was not 130"))
		}
		done <- err
	}()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child never became ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || stderr.String() != "child caught interrupt\n" {
		t.Fatalf("login child did not receive interrupt: error=%v stderr=%q", err, stderr.String())
	}
}
