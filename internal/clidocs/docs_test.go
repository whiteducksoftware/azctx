package clidocs_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/clidocs"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestDocumentationFreshness(t *testing.T) {
	directory := testutil.WorkDir(t)
	if err := clidocs.Sync(directory, false); err != nil {
		t.Fatal(err)
	}
	if err := clidocs.Sync(directory, true); err != nil {
		t.Fatalf("identical second generation is stale: %v", err)
	}
	for _, mutation := range []string{"changed", "missing", "extra"} {
		t.Run(mutation, func(t *testing.T) {
			directory := testutil.WorkDir(t)
			if err := clidocs.Sync(directory, false); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(directory, "azctx.md")
			switch mutation {
			case "changed":
				if err := os.WriteFile(name, []byte("outdated help\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			case "extra":
				if err := os.WriteFile(filepath.Join(directory, "azctx_obsolete.md"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := clidocs.Sync(directory, true); err == nil {
				t.Fatal("stale documentation accepted")
			}
		})
	}
}

func TestCommittedDocumentationIsFresh(t *testing.T) {
	if err := clidocs.Sync("../../docs/cli", true); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentationCanonicalWhitespace(t *testing.T) {
	directory := testutil.WorkDir(t)
	if err := clidocs.Sync(directory, false); err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(directory, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(data, []byte("\n")) || bytes.HasSuffix(data, []byte("\n\n")) || bytes.ContainsRune(data, '\r') {
			t.Fatalf("%s must have canonical LF and exactly one trailing newline", name)
		}
	}
}
