package testutil_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const buildFixtureSource = "package main\nimport \"fmt\"\nfunc main() { fmt.Print(\"isolated fixture\\n\") }\n"

func TestTestMainWithoutGoCanRunNonBuildHelpers(t *testing.T) {
	const child = "AZCTX_TEST_NO_GO_CHILD"
	if os.Getenv(child) == "1" {
		return
	}
	t.Setenv("PATH", "")
	_, output, err := runBuildTestChild(t, "TestTestMainWithoutGoCanRunNonBuildHelpers", child)
	if err != nil {
		t.Fatalf("non-build helper must not require Go: %v\n%s", err, output)
	}
}

func TestBuildConcurrentFirstUseWithIsolatedEnvironment(t *testing.T) {
	dir := testutil.WorkDir(t)
	source := filepath.Join(dir, "main.go")
	writeBuildSource(t, source, buildFixtureSource)
	testutil.IsolateUserDirs(t)
	t.Setenv("PATH", "")
	t.Chdir(dir)
	var ready sync.WaitGroup
	for i := range 4 {
		output := filepath.Join(dir, testutil.ExecutableName("first-use-"+strconv.Itoa(i)))
		ready.Go(func() {
			testutil.Build(t, output, source)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := exec.CommandContext(ctx, output).CombinedOutput()
			if err != nil || string(result) != "isolated fixture\n" {
				t.Errorf("concurrent fixture build: error=%v output=%q", err, result)
			}
		})
	}
	ready.Wait()
}

func TestBuildReusesSnapshotAndCopiesIndependently(t *testing.T) {
	dir := testutil.WorkDir(t)
	source := filepath.Join(dir, "main.go")
	writeBuildSource(t, source, buildFixtureSource)
	first := filepath.Join(dir, testutil.ExecutableName("first"))
	testutil.Build(t, first, source)
	want, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	// A repeated recipe must reuse the package-run snapshot, not invoke Go again.
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(dir, testutil.ExecutableName("second"))
	testutil.Build(t, second, source)
	firstInfo, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Stat(second)
	if err != nil || os.SameFile(firstInfo, secondInfo) {
		t.Fatalf("fixture copies share a file: %v", err)
	}
	if err := os.WriteFile(first, []byte("mutated executable fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(first, 0o600); err != nil {
		t.Fatal(err)
	}

	var ready sync.WaitGroup
	for i := range 4 {
		output := filepath.Join(dir, testutil.ExecutableName("concurrent-"+strconv.Itoa(i)))
		ready.Go(func() {
			testutil.Build(t, output, source)
			if data, err := os.ReadFile(output); err != nil || !bytes.Equal(data, want) {
				t.Errorf("reused copy changed: error=%v", err)
			}
		})
	}
	ready.Wait()
	testutil.Build(t, first, source)
	for _, output := range []string{first, second} {
		assertBuildExecutable(t, output)
	}
}

func assertBuildExecutable(t *testing.T, output string) {
	t.Helper()
	info, err := os.Stat(output)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode: info=%v error=%v", info, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil || string(result) != "isolated fixture\n" {
		t.Fatalf("reused executable: error=%v output=%q", err, result)
	}
}

func TestBuildCachePackageExit(t *testing.T) {
	const child = "AZCTX_TEST_BUILD_CLEANUP_CHILD"
	if os.Getenv(child) == "1" {
		testutil.BuildFakeAZ(t)
		return
	}
	process, output, err := runBuildTestChild(t, "TestBuildCachePackageExit", child)
	if err != nil {
		t.Fatalf("build child: %v\n%s", err, output)
	}
	assertBuildChildCleaned(t, process)
}

func assertBuildChildCleaned(t *testing.T, process *exec.Cmd) {
	t.Helper()
	path := filepath.Join(".test-work", strconv.Itoa(process.Process.Pid))
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("package-exit build cleanup left %s: %v", path, err)
	}
}

func TestBuildFailureCanRetry(t *testing.T) {
	const child = "AZCTX_TEST_BUILD_FAILURE_CHILD"
	if os.Getenv(child) == "1" {
		dir := testutil.WorkDir(t)
		source, output := filepath.Join(dir, "main.go"), filepath.Join(dir, testutil.ExecutableName("fixture"))
		writeBuildSource(t, source, "package main\nthis is not valid Go\n")
		t.Run("invalid", func(t *testing.T) { testutil.Build(t, output, source) })
		writeBuildSource(t, source, buildFixtureSource)
		t.Run("retry", func(t *testing.T) { testutil.Build(t, output, source) })
		return
	}
	process, output, err := runBuildTestChild(t, "TestBuildFailureCanRetry", child)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 ||
		!strings.Contains(string(output), "--- FAIL: TestBuildFailureCanRetry/invalid") ||
		!strings.Contains(string(output), "--- PASS: TestBuildFailureCanRetry/retry") {
		t.Fatalf("failure must surface and a repaired build must retry: error=%v\n%s", err, output)
	}
	assertBuildChildCleaned(t, process)
}

func writeBuildSource(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runBuildTestChild(t *testing.T, name, marker string) (*exec.Cmd, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+name+"$", "-test.v") //nolint:gosec // Re-exec this test binary with fixed test names and no shell.
	process.Env = append(os.Environ(), marker+"=1")
	output, err := process.CombinedOutput()
	return process, output, err
}
