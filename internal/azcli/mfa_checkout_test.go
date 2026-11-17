package azcli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestMFADiagnosticFixturesRetainExactLFOnCRLFCheckout(t *testing.T) {
	dir := testutil.WorkDir(t)
	codes := []string{"50076", "50079", "53003", "other"}
	args := []string{"-c", "core.autocrlf=true", "checkout-index", "--prefix=" + filepath.ToSlash(dir) + "/", "--"}
	for _, code := range codes {
		args = append(args, "internal/azcli/testdata/mfa/"+code+".stderr.golden")
	}
	command := exec.Command("git", args...)
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Git CRLF checkout failed: %v\n%s", err, output)
	}
	for _, code := range codes {
		name := code + ".stderr.golden"
		canonical, err := os.ReadFile(filepath.Join("testdata", "mfa", name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.ContainsRune(canonical, '\r') || bytes.Count(canonical, []byte("\n")) != 1 || !bytes.HasSuffix(canonical, []byte("\n")) {
			t.Fatalf("%s must contain exactly one LF-terminated diagnostic: %q", name, canonical)
		}
		checkedOut, err := os.ReadFile(filepath.Join(dir, "internal", "azcli", "testdata", "mfa", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(checkedOut, canonical) {
			t.Errorf("Git autocrlf checkout changed %s bytes: got %q want %q", name, checkedOut, canonical)
		}
	}
}
