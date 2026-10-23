package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var directoryID atomic.Uint64

// IsolateUserDirs keeps state and cache tests away from the real user's files.
func IsolateUserDirs(t *testing.T) string {
	t.Helper()
	dir := WorkDir(t)
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, dir)
	}
	t.Setenv("AZCTX_CONFIG_DIR", "")
	return dir
}

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

// CRLFCheckout imports current files into an isolated Git index, then checks
// them out with real autocrlf and attribute handling, without using source Git metadata.
func CRLFCheckout(t *testing.T, source string, paths ...string) string {
	t.Helper()
	snapshot, checkout := WorkDir(t), WorkDir(t)
	root, err := os.OpenRoot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, name := range append([]string{".gitattributes"}, paths...) {
		from, to := filepath.Join(source, name), filepath.Join(snapshot, name)
		info, err := os.Stat(from)
		if err != nil {
			t.Fatal(err)
		}
		if info.IsDir() {
			if err := os.CopyFS(to, os.DirFS(from)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := root.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := root.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var gitEnv []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			gitEnv = append(gitEnv, entry)
		}
	}
	gitEnv = append(gitEnv, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
	const configFlag = "-c"
	for _, args := range [][]string{
		{"init", "--quiet", "--template="},
		{configFlag, "core.autocrlf=false", "add", "--force", "--", "."},
		{
			configFlag, "core.autocrlf=true", configFlag, "core.eol=crlf", "checkout-index", "--all",
			"--prefix=" + filepath.ToSlash(checkout) + "/",
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		command := exec.CommandContext(ctx, "git", append([]string{configFlag, "core.hooksPath=" + os.DevNull}, args...)...) //nolint:gosec // Fixed Git operations use only test-owned paths, without a shell.
		command.Dir = snapshot
		command.Env = gitEnv
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("isolated Git CRLF checkout: %v\n%s", err, output)
		}
	}
	return checkout
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
