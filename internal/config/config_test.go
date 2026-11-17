package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const subID = "11111111-1111-1111-1111-111111111111"

const (
	lineOne   = "line 1"
	lineTwo   = "line 2"
	lineThree = "line 3"
)

func TestLoadStrictVersionOne(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	azure := filepath.Join(dir, "azure")
	got, err := config.Load(azure)
	if err != nil || got.Version != 1 || got.Picker != "builtin" || !got.UpdateCheck {
		t.Fatalf("missing config defaults = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		name, yaml, diagnostic string
	}{
		{"valid", "version: 1\naliases:\n  prod: " + subID + "\npinned: [" + subID + "]\nupdateCheck: false\npicker: fzf\n", ""},
		{"unknown", "version: 1\nupdatCheck: false\n", lineTwo},
		{"nested unknown", "version: 1\npicker:\n  backend: builtin\n", lineThree},
		{"corrupt", "version: 1\naliases: [\n", "line"},
		{"version", "version: 2\n", lineOne},
		{"missing version", "picker: builtin\n", lineOne},
		{"string version", "version: \"1\"\n", lineOne},
		{"string bool", "version: 1\nupdateCheck: \"false\"\n", lineTwo},
		{"null", "version: 1\nhidden: null\n", lineTwo},
		{"invalid ID", "version: 1\nprotected:\n  - invalid\n", lineThree},
		{"invalid alias ID", "version: 1\naliases:\n  prod: invalid\n", lineThree},
		{"duplicate", "version: 1\nversion: 1\n", lineTwo},
		{"duplicate alias", "version: 1\naliases:\n  prod: " + subID + "\n  PROD: " + subID + "\n", "line 4"},
		{"Unicode duplicate alias", "version: 1\naliases:\n  S: " + subID + "\n  ſ: " + subID + "\n", "line 4"},
		{"picker", "version: 1\npicker: other\n", lineTwo},
		{"extra document", "version: 1\n---\nversion: 1\n", lineThree},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := config.Load(azure)
			if tc.diagnostic != "" {
				if err == nil || !strings.Contains(err.Error(), tc.diagnostic) || !strings.Contains(err.Error(), "config.yaml") {
					t.Fatalf("diagnostic = %v, want path and %s", err, tc.diagnostic)
				}
			} else if err != nil || got.Aliases["prod"] != subID || got.UpdateCheck || got.Picker != "fzf" {
				t.Fatalf("config = %+v, %v", got, err)
			}
		})
	}
}
