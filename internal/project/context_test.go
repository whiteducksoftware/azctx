package project_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/project"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const projectFixture = "version: 1\nsubscription: 11111111-1111-1111-1111-111111111111\ntenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa\n"

func TestNearestProjectStrictReadOnlyBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, data, diagnostic string
	}{
		{"valid", projectFixture, ""},
		{"empty", "", "line 1"},
		{"quoted version", strings.Replace(projectFixture, "version: 1", `version: "1"`, 1), "version must be 1"},
		{"missing subscription", "version: 1\ntenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa\n", "subscription is required"},
		{"null tenant", strings.Replace(projectFixture, "tenant: aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "tenant: null", 1), "tenant must be a GUID"},
		{"alias", "version: 1\nsubscription: &id 11111111-1111-1111-1111-111111111111\ntenant: *id\n", "tenant must be a GUID"},
		{"merge", projectFixture + "<<: {}\n", "invalid key"},
		{"syntax", projectFixture + "unexpected: [\n", "invalid project YAML"},
		{"extra document line", projectFixture + "---\n", "line 4: only one YAML document"},
		{"oversized", projectFixture + strings.Repeat(" ", 65536), "exceeds 65536"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.WorkDir(t)
			path := filepath.Join(dir, project.Filename)
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(dir, "nested")
			if err := os.Mkdir(nested, 0o700); err != nil {
				t.Fatal(err)
			}
			got, found, err := project.Find(nested)
			if tc.diagnostic == "" {
				if err != nil || found != path || !got.Matches("11111111-1111-1111-1111-111111111111", "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA") {
					t.Fatalf("context=%+v path=%s error=%v", got, found, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.diagnostic) || !strings.Contains(err.Error(), path) {
				t.Fatalf("expected diagnostic %q with filename: %v", tc.diagnostic, err)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || string(after) != tc.data {
				t.Fatalf("project file mutated: %q, %v", after, readErr)
			}
		})
	}
}

func TestProjectMissingAndInvalidNearestNeverFallBack(t *testing.T) {
	dir := testutil.WorkDir(t)
	if _, _, err := project.Find(dir); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	parent := filepath.Join(dir, project.Filename)
	if err := os.WriteFile(parent, []byte(projectFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			child := filepath.Join(dir, kind)
			if err := os.Mkdir(child, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(child, project.Filename)
			if kind == "directory" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(parent, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, found, err := project.Find(child); err == nil || found != path || !strings.Contains(err.Error(), "regular file") {
				t.Fatalf("invalid nearest project fell back: path=%s error=%v", found, err)
			}
		})
	}
}
