package azcli_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type cancelOnOutput struct {
	cancel context.CancelFunc
	input  *io.PipeWriter
}

func (w cancelOnOutput) Write(data []byte) (int, error) {
	w.cancel()
	_ = w.input.Close()
	return len(data), nil
}

func TestRunnerPreservesActiveContextCancellation(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	fixtures := filepath.Join(dir, "fixtures.json")
	data := `[{"args":["login"],"stdout":"started\n","waitForInput":true}]`
	if err := os.WriteFile(fixtures, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKEAZ_FIXTURES", fixtures)
	t.Setenv("FAKEAZ_RECORD", filepath.Join(dir, "calls.jsonl"))
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, output := io.Pipe()
	defer input.Close()
	defer output.Close()
	result, err := runner.Run(ctx, []string{loginCommand}, azcli.Options{
		Terminal: &azcli.Terminal{Stdin: input, Stdout: cancelOnOutput{cancel, output}, Stderr: io.Discard},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("active cancellation cause lost: result=%+v error=%v", result, err)
	}
}
