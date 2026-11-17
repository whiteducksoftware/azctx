package testutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

var packageBuilds struct {
	sync.Mutex
	dir, cwd, goTool string
	goErr            error
	env              []string
	binaries         map[string]string
}

// RunTests owns default fixture builds for one test process. Call it from
// TestMain, passing its result to os.Exit only after build cleanup has run.
func RunTests(m *testing.M) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	packageBuilds.dir = filepath.Join(cwd, ".test-work", strconv.Itoa(os.Getpid()), "builds")
	packageBuilds.cwd = cwd
	packageBuilds.goTool, packageBuilds.goErr = exec.LookPath("go")
	packageBuilds.env = os.Environ()
	packageBuilds.binaries = make(map[string]string)
	code := m.Run()
	if err := os.RemoveAll(packageBuilds.dir); err != nil {
		fmt.Fprintf(os.Stderr, "clean up fixture builds: %v\n", err)
		code = 1
	}
	// Other test processes may still own siblings below .test-work.
	_ = os.Remove(filepath.Dir(packageBuilds.dir))
	_ = os.Remove(filepath.Dir(filepath.Dir(packageBuilds.dir)))
	return code
}

func cachedBuild(pkg string) (string, error) {
	packageBuilds.Lock()
	defer packageBuilds.Unlock()
	if packageBuilds.binaries == nil {
		return "", errors.New("fixture builds require testutil.RunTests in TestMain")
	}
	if binary, ok := packageBuilds.binaries[pkg]; ok {
		return binary, nil
	}
	if packageBuilds.goErr != nil {
		return "", packageBuilds.goErr
	}
	if err := os.MkdirAll(packageBuilds.dir, 0o700); err != nil {
		return "", err
	}
	binary := filepath.Join(packageBuilds.dir, ExecutableName(strconv.Itoa(len(packageBuilds.binaries))))
	command := exec.Command(packageBuilds.goTool, "build", "-o", binary, pkg) //nolint:gosec // The active Go compiler and fixture recipe are test-owned; no shell is used.
	command.Dir, command.Env = packageBuilds.cwd, packageBuilds.env
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(binary)
		return "", fmt.Errorf("build %s: %w\n%s", pkg, err, output)
	}
	packageBuilds.binaries[pkg] = binary
	return binary, nil
}

// Build reuses the default build of pkg, copying bytes (never links) to output.
// Compiler cwd/environment are fixed at package start; test environments and
// mutable executable copies remain independent. Custom-flag builds stay separate.
func Build(t *testing.T, output, pkg string) {
	t.Helper()
	binary, err := cachedBuild(pkg)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(output, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(errors.Join(err, source.Close()))
	}
	_, copyErr := io.Copy(destination, source)
	if err := errors.Join(copyErr, destination.Chmod(0o755), destination.Close(), source.Close()); err != nil {
		t.Fatalf("copy build %s: %v", pkg, err)
	}
}
