package cmd_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestEnvironmentGitHubAppendsInPlace(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	deps.Runner = nil
	path := filepath.Join(testutil.WorkDir(t), envPipelineFile)
	if err := os.WriteFile(path, []byte("EXISTING=value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_ENV", path)
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var out, messages bytes.Buffer
	if code := cmd.Run([]string{envCommand, formatFlag, envGitHubFormat}, deps, strings.NewReader(""), &out, &messages); code != 0 || out.Len() != 0 || messages.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &messages)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || (runtime.GOOS != platformWindows && after.Mode().Perm() != 0o644) {
		t.Fatalf("GITHUB_ENV replaced: same=%v mode %v->%v", os.SameFile(before, after), before.Mode(), after.Mode())
	}
	want, err := os.ReadFile(filepath.Join("testdata", "env", envGitHubFormat+".golden"))
	if err != nil {
		t.Fatal(err)
	}
	if seen, err := io.ReadAll(reader); err != nil || !bytes.Equal(seen, want) {
		t.Fatalf("runner handle saw %q, %v; want %q", seen, err, want)
	}
}
