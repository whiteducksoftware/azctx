package azcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestRunnerRedactsAttachedAndCapturedStderr(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	message := "access_token=" + "synthetic-secret" + "\nUse code SYNTHETIC\n"
	fixtures, err := json.Marshal([]map[string]any{
		{"args": []string{loginCommand}, "stderr": message},
		{"args": []string{accountCommand, showCommand}, "stderr": message},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixtures.json")
	if err := os.WriteFile(path, fixtures, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKEAZ_FIXTURES", path)
	t.Setenv("FAKEAZ_RECORD", filepath.Join(dir, "calls.jsonl"))
	t.Setenv("FAKEAZ_ENV_KEYS", "")
	runner, err := azcli.NewRunner()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	for _, options := range []azcli.Options{{Terminal: &azcli.Terminal{Stderr: &stderr}}, {}} {
		args := []string{accountCommand, showCommand}
		if options.Terminal != nil {
			args = []string{loginCommand}
		}
		result, err := runner.Run(context.Background(), args, options)
		if err != nil || strings.Contains(result.Stderr, "synthetic-secret") ||
			!strings.Contains(result.Stderr, "Use code SYNTHETIC\n") || !strings.Contains(result.Stderr, "[REDACTED]") {
			t.Fatalf("unsafe captured diagnostic: %+v, %v", result, err)
		}
	}
	if strings.Contains(stderr.String(), "synthetic-secret") || !strings.Contains(stderr.String(), "Use code SYNTHETIC\n") {
		t.Fatalf("unsafe attached diagnostic: %q", stderr.String())
	}
}

func TestRunnerRefusesCurrentDirectoryExecutable(t *testing.T) {
	dir := testutil.BuildFakeAZ(t)
	t.Chdir(dir)
	t.Setenv("PATH", ".")
	t.Setenv("GODEBUG", "execerrdot=1")
	if _, err := azcli.NewRunner(); !errors.Is(err, exec.ErrDot) {
		t.Fatalf("current-directory az must retain Go's refusal: %v", err)
	}
}
