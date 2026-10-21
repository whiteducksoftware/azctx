package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
)

var directoryID atomic.Uint64

// WorkDir allocates scratch space below the package's working directory, never
// the host temporary directory. It is removed when the test finishes.
func WorkDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(".test-work", strconv.Itoa(os.Getpid()), strconv.FormatUint(directoryID.Add(1), 10)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
		// Parent removals succeed only when no other tests are using them.
		_ = os.Remove(filepath.Dir(dir))
		_ = os.Remove(filepath.Dir(filepath.Dir(dir)))
	})
	return dir
}

func ExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// BuildFakeAZ builds the portable fixture process with the active Go toolchain.
func BuildFakeAZ(t *testing.T) string {
	t.Helper()
	dir := WorkDir(t)
	Build(t, filepath.Join(dir, ExecutableName("az")), "github.com/whiteducksoftware/azctx/internal/testutil/fakeaz")
	return dir
}

func Build(t *testing.T, output, pkg string) {
	t.Helper()
	command := exec.Command("go", "build", "-o", output, pkg)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, output)
	}
}
