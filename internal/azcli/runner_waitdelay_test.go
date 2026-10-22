package azcli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const (
	pipeSuccessOutcome = "success"
	pipeProcessError   = "process error"
	pipeFlushError     = "flush error"
)

func TestSuccessfulLoginWithInheritedStderrPipe(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("az")), "github.com/whiteducksoftware/azctx/internal/testutil/contextchild")
	t.Setenv("PATH", dir)
	t.Setenv("AZCTX_TEST_CHILD_MODE", "az-pipe")
	failure := errors.New("terminal flush failed")
	for _, outcome := range []string{pipeSuccessOutcome, pipeProcessError, pipeFlushError} {
		t.Run(outcome, func(t *testing.T) {
			ready := filepath.Join(testutil.WorkDir(t), "ready")
			t.Setenv("AZCTX_TEST_CHILD_READY", ready)
			t.Setenv("AZCTX_TEST_CHILD_NO_NEWLINE", "1")
			t.Setenv("AZCTX_TEST_CHILD_EXIT", "")
			if outcome == pipeProcessError {
				t.Setenv("AZCTX_TEST_CHILD_EXIT", "1")
			}
			runner, err := azcli.NewRunner()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var stderr bytes.Buffer
			var terminal io.Writer = &stderr
			if outcome == pipeFlushError {
				terminal = failedWriter{failure}
			}
			result, runErr := runner.Run(ctx, []string{loginCommand}, azcli.Options{
				Terminal: &azcli.Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: terminal},
			})
			killPipeDescendant(t, ready)
			if result.Stderr != "login completed" || ctx.Err() != nil {
				t.Fatalf("login: result=%+v error=%v context=%v", result, runErr, ctx.Err())
			}
			assertInheritedPipeResult(t, outcome, result, runErr, failure, stderr.String())
		})
	}
}

func assertInheritedPipeResult(t *testing.T, outcome string, result azcli.Result, err, failure error, stderr string) {
	t.Helper()
	switch outcome {
	case pipeSuccessOutcome:
		if err != nil || result.ExitCode != 0 || stderr != result.Stderr {
			t.Fatalf("successful login: result=%+v error=%v terminal=%q", result, err, stderr)
		}
	case pipeProcessError:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || result.ExitCode != 7 || exit.ExitCode() != 7 {
			t.Fatalf("real process error lost: result=%+v error=%v", result, err)
		}
	case pipeFlushError:
		if !errors.Is(err, failure) || result.ExitCode != 0 {
			t.Fatalf("flush error lost: result=%+v error=%v", result, err)
		}
	}
}

func killPipeDescendant(t *testing.T, ready string) {
	t.Helper()
	data, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Release() }()
	if err := process.Kill(); err != nil {
		t.Error(err)
	}
}
