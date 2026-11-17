package cmd_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestBinaryFixtureCopiesAreIsolated(t *testing.T) {
	var previous []string
	for _, name := range []string{"mutated", "fresh"} {
		t.Run(name, func(t *testing.T) {
			binary, azure, record := fakeBinary(t, `[
				{"args":["version","-o","json"],"stdout":"{\"azure-cli\":\"2.61.0\"}"},
				{"args":["login","--use-device-code"]}
			]`)
			home, fixtures := os.Getenv("HOME"), filepath.Join(filepath.Dir(binary), "fixtures.json")
			if os.Getenv("FAKEAZ_FIXTURES") != fixtures {
				t.Fatal("fake az did not receive the test-owned fixtures")
			}
			paths := []string{binary, azure, record, home, fixtures}
			if name == "mutated" {
				previous = paths
				for _, path := range []string{binary, filepath.Join(filepath.Dir(binary), testutil.ExecutableName("az"))} {
					writeFixture(t, path, "mutated test-owned executable")
				}
				writeFixture(t, fixtures, "[]")
				writeFixture(t, record, "used fixture calls\n")
				writeFixture(t, filepath.Join(azure, profileFile), "{}")
				return
			}
			for i, path := range paths {
				if path == previous[i] {
					t.Fatalf("mutable test path reused: %s", path)
				}
			}
			if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("call log was not isolated: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, binary, loginCommand, argumentDelimiter, deviceCodeFlag).CombinedOutput()
			if err != nil || !strings.Contains(string(output), "✓ Signed in: 1 tenants, 2 subscriptions\n") {
				t.Fatalf("fixture mutation escaped its test: error=%v output=%q", err, output)
			}
			if calls, err := os.ReadFile(record); err != nil || strings.Count(string(calls), "\n") != 2 {
				t.Fatalf("fresh call log=%s error=%v", calls, err)
			}
		})
	}
}
