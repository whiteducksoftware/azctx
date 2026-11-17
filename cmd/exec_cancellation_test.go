package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestExecCancellationTerminatesUncooperativeChild(t *testing.T) {
	dir := testutil.WorkDir(t)
	helper := filepath.Join(dir, testutil.ExecutableName(childFixtureName))
	testutil.Build(t, helper, childFixturePackage)
	deps := commandDependencies(t, &testutil.FakeRunner{})
	t.Setenv(childModeEnv, "ignore")
	ready := filepath.Join(dir, "ready")
	t.Setenv(childReadyEnv, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	command := cmd.NewRootCommand(deps)
	command.SetContext(ctx)
	command.SetArgs([]string{execSubcommand, productionID, argumentDelimiter, helper})
	var out, messages bytes.Buffer
	command.SetIn(strings.NewReader(""))
	command.SetOut(&out)
	command.SetErr(&messages)
	done := make(chan error, 1)
	go func() { done <- command.Execute() }()
	waitExecChild(ctx, t, ready)
	cancel()
	select {
	case err := <-done:
		if cmd.ExitCode(err) != 130 || !errors.Is(err, context.Canceled) || out.Len() != 0 || messages.Len() != 0 {
			t.Fatalf("error=%v code=%d stdout=%q stderr=%q", err, cmd.ExitCode(err), &out, &messages)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("uncooperative child outlived the cancellation fallback")
	}
}

func waitExecChild(ctx context.Context, t *testing.T, path string) int {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil {
				return pid
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("helper never became ready")
		case <-ticker.C:
		}
	}
}
