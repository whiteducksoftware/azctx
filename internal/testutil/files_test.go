package testutil_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

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
