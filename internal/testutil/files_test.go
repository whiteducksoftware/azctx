package testutil_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestCRLFCheckoutIgnoresCallerGitConfiguration(t *testing.T) {
	for _, name := range []string{
		"global configuration and templates", "inherited repository and index", "inherited worktree and command configuration",
	} {
		t.Run(name, func(t *testing.T) {
			decoy, source, settings := testutil.WorkDir(t), testutil.WorkDir(t), testutil.WorkDir(t)
			runCheckoutGit(t, "init", "--quiet", "--template=", decoy)
			writeCheckoutFile(t, decoy, "keep.txt", "decoy working file\n")
			runCheckoutGit(t, "-C", decoy, "add", "keep.txt")
			before := checkoutFiles(t, decoy)
			t.Cleanup(func() {
				if after := checkoutFiles(t, decoy); !reflect.DeepEqual(after, before) {
					t.Error("caller repository files, configuration or index changed")
				}
			})
			writeCheckoutFile(t, source, ".gitattributes", "/generated.md text eol=lf\n")
			writeCheckoutFile(t, source, "README.md", "current working-tree README\n")
			writeCheckoutFile(t, source, "generated.md", "current untracked page\n")
			writeCheckoutFile(t, settings, "attributes", "* -text filter=decoy\n")
			writeCheckoutFile(t, settings, "config", "[core]\nattributesFile = "+filepath.ToSlash(filepath.Join(settings, "attributes"))+
				"\nhooksPath = "+filepath.ToSlash(decoy)+"\n[init]\ntemplateDir = "+filepath.ToSlash(decoy)+
				"\n[filter \"decoy\"]\nclean = git azctx-test-filter-must-not-run\nrequired = true\n")
			t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(settings, "config"))
			t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(settings, "config"))
			if name != "global configuration and templates" {
				t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
				t.Setenv("GIT_INDEX_FILE", filepath.Join(decoy, ".git", "index"))
			}
			if name == "inherited worktree and command configuration" {
				for key, value := range map[string]string{
					"GIT_WORK_TREE": decoy, "GIT_COMMON_DIR": filepath.Join(decoy, ".git"),
					"GIT_OBJECT_DIRECTORY": filepath.Join(decoy, ".git", "objects"),
					"GIT_CONFIG_COUNT":     "1", "GIT_CONFIG_KEY_0": "core.worktree", "GIT_CONFIG_VALUE_0": decoy,
				} {
					t.Setenv(key, value)
				}
			}
			checkout := testutil.CRLFCheckout(t, source, "README.md", "generated.md")
			for file, want := range map[string]string{
				"README.md": "current working-tree README\r\n", "generated.md": "current untracked page\n",
			} {
				data, err := os.ReadFile(filepath.Join(checkout, file))
				if err != nil || string(data) != want {
					t.Errorf("%s: got %q, want %q, error %v", file, data, want, err)
				}
			}
		})
	}
}

func runCheckoutGit(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = gitEnvironmentForDecoy()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("seed checkout repository: %v\n%s", err, output)
	}
}

func gitEnvironmentForDecoy() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
}

func writeCheckoutFile(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func checkoutFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	files := make(map[string]string)
	if err := fs.WalkDir(root.FS(), ".", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := root.ReadFile(path)
		files[path] = string(data)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCRLFCheckoutUsesWorkingTree(t *testing.T) {
	for _, repository := range []bool{false, true} {
		name := "source without Git"
		if repository {
			name = "unstaged and untracked"
		}
		t.Run(name, func(t *testing.T) {
			source := testutil.WorkDir(t)
			write := func(name, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(source, name), []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(".gitattributes", "/generated.md text eol=lf\n")
			write("README.md", "old indexed README\n")
			if repository {
				for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}} {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					command := exec.CommandContext(ctx, "git", args...)
					command.Dir = source
					command.Env = gitEnvironmentForDecoy()
					output, err := command.CombinedOutput()
					cancel()
					if err != nil {
						t.Fatalf("seed index: %v\n%s", err, output)
					}
				}
			}
			write("README.md", "working-tree README\nsecond line\n")
			write("generated.md", "new generated page\n")
			checkout := testutil.CRLFCheckout(t, source, "README.md", "generated.md")
			for name, want := range map[string]string{
				"README.md":    "working-tree README\r\nsecond line\r\n",
				"generated.md": "new generated page\n",
			} {
				data, err := os.ReadFile(filepath.Join(checkout, name))
				if err != nil || !bytes.Equal(data, []byte(want)) {
					t.Errorf("%s: got %q, want %q, error %v", name, data, want, err)
				}
			}
			data, err := os.ReadFile(filepath.Join(source, "README.md"))
			if err != nil || string(data) != "working-tree README\nsecond line\n" {
				t.Fatalf("source was changed: %q, %v", data, err)
			}
		})
	}
}
