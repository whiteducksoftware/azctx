package update_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/update"
)

const statusAvailable = "available"

const cachedStableRelease = `{"checkedAt":"2026-10-21T10:00:00Z","tag":"v2026.11.0"}`

func TestCachedUpdateStatusIsReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name, cache, current, state, latest string
		stale, failure                      bool
	}{
		{name: "missing", current: oldVersion, state: "not_checked"},
		{name: "invalid", cache: "{", current: oldVersion, state: "unavailable", failure: true},
		{name: "failed", cache: `{"checkedAt":"2026-10-21T10:00:00Z","tag":""}`, current: oldVersion, state: "check_failed"},
		{name: "newer", cache: cachedStableRelease, current: oldVersion, state: statusAvailable, latest: stableVersion},
		{name: "same", cache: cachedStableRelease, current: stableVersion, state: "current", latest: stableVersion},
		{name: "stale", cache: `{"checkedAt":"2026-10-19T10:00:00Z","tag":"v2026.11.0"}`, current: oldVersion, state: statusAvailable, latest: stableVersion, stale: true},
		{name: "future", cache: `{"checkedAt":"2026-10-22T10:00:00Z","tag":"v2026.11.0"}`, current: oldVersion, state: statusAvailable, latest: stableVersion, stale: true},
		{name: "development", cache: cachedStableRelease, current: "(devel)", state: "unknown_version", latest: stableVersion},
		{name: "unsafe-tag", cache: `{"checkedAt":"2026-10-21T10:00:00Z","tag":"v2026.11.0+secret-tag"}`, current: oldVersion, state: statusAvailable, latest: stableVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.IsolateUserDirs(t)
			azureDir := filepath.Join(dir, "azure")
			name := filepath.Join(dir, "azctx", "update.json")
			if tc.cache != "" {
				if err := storage.Write(name, azureDir, []byte(tc.cache)); err != nil {
					t.Fatal(err)
				}
			}
			checker := update.Checker{
				Current: tc.current, CacheDir: dir, AzureDir: azureDir,
				Now: func() time.Time { return time.Date(2026, 10, 21, 11, 0, 0, 0, time.UTC) },
			}
			status, err := checker.CachedStatus()
			if (err != nil) != tc.failure || status.State != tc.state || status.LatestVersion != tc.latest || status.Stale != tc.stale ||
				strings.Contains(status.LatestVersion, "secret") {
				t.Fatalf("status=%+v error=%v", status, err)
			}
			data, readErr := os.ReadFile(name)
			if string(data) != tc.cache || (tc.cache == "" && !os.IsNotExist(readErr)) {
				t.Fatalf("cache changed: %q, %v", data, readErr)
			}
			if _, err := os.Stat(name + ".lock"); !os.IsNotExist(err) {
				t.Fatalf("read-only status created a lock: %v", err)
			}
		})
	}
}
