package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestConfigStorageIsolationAndFailures(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	azure := filepath.Join(dir, "azure")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if path, err := config.Path(azure); err != nil || path != filepath.Join(base, "azctx", "config.yaml") {
		t.Fatalf("default path = %s, %v", path, err)
	}
	owned := filepath.Join(dir, "private")
	t.Setenv("AZCTX_CONFIG_DIR", owned)
	cfg := config.Default()
	cfg.Aliases["prod"] = subID
	if err := config.Save(azure, cfg); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(owned, "config.yaml")
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{owned: 0o700, path: 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("private permissions: %s, %v, %v", path, info, err)
			}
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Aliases["prod"] = "not-an-ID"
	if err := config.Save(azure, cfg); err == nil {
		t.Fatal("invalid aliases must fail without overwriting")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed save changed configuration: %q, %v", after, err)
	}
}

func TestConfigReadAndWriteFailures(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	owned := filepath.Join(dir, "private")
	t.Setenv("AZCTX_CONFIG_DIR", owned)
	azure := filepath.Join(dir, "azure")
	path := filepath.Join(owned, "config.yaml")
	if err := os.MkdirAll(owned, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(azure); err == nil || !strings.Contains(err.Error(), "read ") {
		t.Fatalf("read failure not surfaced: %v", err)
	}
	if err := config.Save(azure, config.Default()); err == nil || !strings.Contains(err.Error(), "write ") {
		t.Fatalf("write failure not surfaced: %v", err)
	}
	entries, err := os.ReadDir(owned)
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed write left staging files: %v, %v", entries, err)
	}
}

func TestConfigOverrideRejectsAzureOverlapBeforeReadingOrWriting(t *testing.T) {
	for _, mode := range []string{"direct", "nested", "environment", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			dir := testutil.IsolateUserDirs(t)
			azure := filepath.Join(dir, "azure")
			if err := os.Mkdir(azure, 0o700); err != nil {
				t.Fatal(err)
			}
			target, explicit := azure, azure
			switch mode {
			case "nested":
				target = filepath.Join(azure, "azctx")
			case "environment":
				t.Setenv("AZURE_CONFIG_DIR", azure)
				explicit = filepath.Join(dir, "other-azure")
			case "symlink":
				target = filepath.Join(dir, "link")
				if err := os.Symlink(azure, target); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("AZCTX_CONFIG_DIR", target)
			if _, err := config.Load(explicit); err == nil || !strings.Contains(err.Error(), "inside Azure") {
				t.Fatalf("unsafe read accepted: %v", err)
			}
			if err := config.Save(explicit, config.Default()); err == nil {
				t.Fatal("unsafe write accepted")
			}
			entries, err := os.ReadDir(azure)
			if err != nil || len(entries) != 0 {
				t.Fatalf("Azure directory was touched: %v, %v", entries, err)
			}
		})
	}
}
