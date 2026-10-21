package state_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const subscriptionID = "11111111-1111-1111-1111-111111111111"

func TestPreviousStatePrivateRoundTrip(t *testing.T) {
	home := testutil.IsolateUserDirs(t)
	azure := filepath.Join(home, ".azure")
	if _, err := state.Load(azure); err == nil {
		t.Fatal("missing previous state must fail")
	}
	if err := state.Save(azure, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if got, err := state.Load(azure); err != nil || got != subscriptionID {
		t.Fatalf("previous = %q, %v", got, err)
	}
	if err := state.Save(azure, "not-a-guid"); err == nil {
		t.Fatal("invalid subscription must not overwrite valid state")
	}
	if got, err := state.Load(azure); err != nil || got != subscriptionID {
		t.Fatalf("previous after rejected write = %q, %v", got, err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(config, "azctx")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("state directory = %v, %v", entries, err)
	}
	assertPrivatePermissions(t, dir)
}

func assertPrivatePermissions(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "state.json"): 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("permissions on %s: %v, %v", path, info, err)
			}
		}
	}
}

func TestStateNeverWritesInsideAzureConfig(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "overlapping", true: "nested"}[nested], func(t *testing.T) {
			testutil.IsolateUserDirs(t)
			config, err := os.UserConfigDir()
			if err != nil {
				t.Fatal(err)
			}

			azure := filepath.Join(config, "azctx")
			if nested {
				azure = config
			}
			if err := state.Save(azure, subscriptionID); err == nil {
				t.Fatal("state must reject an Azure configuration destination")
			}
			if _, err := os.Stat(filepath.Join(config, "azctx")); !os.IsNotExist(err) {
				t.Fatalf("rejected storage created a directory: %v", err)
			}
		})
	}
}

func TestStateRejectsSymlinkIntoAzure(t *testing.T) {
	testutil.IsolateUserDirs(t)
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	azure := filepath.Join(testutil.WorkDir(t), "azure")
	if err := os.MkdirAll(azure, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(azure, filepath.Join(config, "azctx")); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(azure, subscriptionID); err == nil {
		t.Fatal("symlink storage inside Azure was accepted")
	}
	entries, err := os.ReadDir(azure)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Azure directory modified: %v, %v", entries, err)
	}
}
