package update_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/update"
	"github.com/whiteducksoftware/azctx/log"
)

const (
	oldVersion    = "2024.12.5"
	stableVersion = "2026.11.0"
	latestTag     = "v" + stableVersion
)

func testChecker(t *testing.T, server *httptest.Server, output *bytes.Buffer) update.Checker {
	t.Helper()
	home := testutil.IsolateUserDirs(t)
	t.Setenv("CI", "")
	if err := os.Unsetenv("CI"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZCTX_NO_UPDATE_CHECK", "")
	return update.Checker{
		URL: server.URL + "/owner/repo/releases/latest", Current: oldVersion,
		CacheDir: home, AzureDir: filepath.Join(home, ".azure"),
		IsTerminal: func() bool { return true }, Output: output,
	}
}

func TestDailyCachedReleaseRedirect(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s", r.Method)
		}
		if strings.HasSuffix(r.URL.Path, "/latest") {
			requests.Add(1)
			http.Redirect(w, r, "/owner/repo/releases/tag/"+latestTag, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	var output bytes.Buffer
	checker := testChecker(t, server, &output)
	now := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	checker.Now = func() time.Time { return now }
	for range 2 {
		if err := checker.Check("owner", "repo"); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 1 || !strings.Contains(output.String(), "2024.12.5 → "+latestTag) {
		t.Fatalf("requests=%d notice=%q", requests.Load(), output.String())
	}
	now = now.Add(24 * time.Hour)
	if err := checker.Check("owner", "repo"); err != nil || requests.Load() != 2 {
		t.Fatalf("next-day requests=%d error=%v", requests.Load(), err)
	}
}

func TestFutureUpdateCacheExpiresWithClockRollback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		future time.Duration
		failed bool
	}{
		{name: "one nanosecond", future: time.Nanosecond},
		{name: "two days", future: 48 * time.Hour},
		{name: "failed attempt", future: 48 * time.Hour, failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, requests := futureReleaseServer(tc.failed)
			defer server.Close()
			var output bytes.Buffer
			checker := testChecker(t, server, &output)
			present := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
			now := present.Add(tc.future)
			checker.Now = func() time.Time { return now }
			if err := checker.Check("owner", "repo"); (err != nil) != tc.failed {
				t.Fatalf("seed future cache: %v", err)
			}
			now = present
			status, err := checker.CachedStatus()
			if err != nil || !status.Stale {
				t.Fatalf("future cache status=%+v err=%v", status, err)
			}
			output.Reset()
			if err := checker.Check("owner", "repo"); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 2 || !strings.Contains(output.String(), "v2026.12.0") {
				t.Fatalf("future cache suppressed refresh: requests=%d notice=%q", requests.Load(), &output)
			}
			status, err = checker.CachedStatus()
			if err != nil || status.Stale || status.State != statusAvailable || status.LatestVersion != "2026.12.0" {
				t.Fatalf("refreshed cache status=%+v err=%v", status, err)
			}
			if err := checker.Check("owner", "repo"); err != nil || requests.Load() != 2 {
				t.Fatalf("refreshed cache not reused: requests=%d err=%v", requests.Load(), err)
			}
		})
	}
}

func futureReleaseServer(failFirst bool) (*httptest.Server, *atomic.Int32) {
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/latest") {
			count := requests.Add(1)
			if count == 1 && failFirst {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			tag := latestTag
			if count > 1 {
				tag = "v2026.12.0"
			}
			http.Redirect(w, r, "/owner/repo/releases/tag/"+tag, http.StatusFound)
		}
	}))
	return server, requests
}

func TestUpdateEnvironmentAndTerminalGates(t *testing.T) {
	for _, gate := range []string{"pipe", "CI", "empty CI", "AZCTX_NO_UPDATE_CHECK"} {
		t.Run(gate, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("gated check contacted the server")
			}))
			defer server.Close()
			var output bytes.Buffer
			checker := testChecker(t, server, &output)
			switch gate {
			case "pipe":
				checker.IsTerminal = func() bool { return false }
			case "CI":
				t.Setenv(gate, "false")
			case "empty CI":
				t.Setenv("CI", "")
			default:
				t.Setenv(gate, "1")
			}
			if err := checker.Check("owner", "repo"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(checker.CacheDir, "azctx")); !os.IsNotExist(err) {
				t.Fatalf("gated check touched cache: %v", err)
			}
		})
	}
}

func TestUpdateDeadlineAndFailedAttemptCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("custom transport must not contact a server")
	}))
	defer server.Close()
	var output bytes.Buffer
	checker := testChecker(t, server, &output)
	var requests int
	checker.Client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Errorf("missing <=2 second deadline: %v, %v", deadline, ok)
		}
		return nil, context.DeadlineExceeded
	})}
	if err := checker.Check("owner", "repo"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	if err := checker.Check("owner", "repo"); err != nil || requests != 1 || output.Len() != 0 {
		t.Fatalf("failed-attempt cache: requests=%d error=%v notice=%q", requests, err, output.String())
	}
}

func TestConcurrentUpdateCheckDoesNotDuplicateRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("another check owns the update lock")
	}))
	defer server.Close()
	var output bytes.Buffer
	checker := testChecker(t, server, &output)
	lockPath, err := storage.Path(checker.CacheDir, checker.AzureDir, "update.json.lock")
	if err != nil {
		t.Fatal(err)
	}
	release, acquired, err := storage.Lock(lockPath, checker.AzureDir)
	if err != nil || !acquired {
		t.Fatalf("lock: acquired=%v error=%v", acquired, err)
	}
	defer release()
	if err := checker.Check("owner", "repo"); err != nil || output.Len() != 0 {
		t.Fatalf("concurrent check: %v, %q", err, output.String())
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCalVerAndPrereleaseNotices(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		notice          bool
	}{
		{oldVersion, latestTag, true},
		{"2026.9.9", stableVersion, true},
		{"2026.11.9", "2026.11.10", true},
		{"2026.11.10", "2026.11.9", false},
		{stableVersion, stableVersion, false},
		{oldVersion, "2026.11.0-rc.1", false},
		{"2026.11.0-rc.1", stableVersion, true},
		{"unknown", latestTag, false},
		{oldVersion, "not-a-release", false},
	} {
		t.Run(tc.current+"_"+tc.latest, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/latest") {
					http.Redirect(w, r, "/owner/repo/releases/tag/"+tc.latest, http.StatusFound)
				}
			}))
			defer server.Close()
			var output bytes.Buffer
			checker := testChecker(t, server, &output)
			checker.Current = tc.current
			if err := checker.Check("owner", "repo"); err != nil || (output.Len() != 0) != tc.notice {
				t.Fatalf("notice=%q error=%v want notice=%v", output.String(), err, tc.notice)
			}
		})
	}
}

func TestUpdateCacheNeverWritesInsideAzure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unsafe cache must not contact server")
	}))
	defer server.Close()
	var output bytes.Buffer
	checker := testChecker(t, server, &output)
	checker.AzureDir = checker.CacheDir
	if err := checker.Check("owner", "repo"); err == nil {
		t.Fatal("cache inside Azure configuration was accepted")
	}
	if _, err := os.Stat(filepath.Join(checker.CacheDir, "azctx")); !os.IsNotExist(err) {
		t.Fatalf("unsafe cache was created: %v", err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	checker.CacheDir, checker.AzureDir = "", cacheDir
	if err := checker.Check("owner", "repo"); err == nil {
		t.Fatal("default user-cache directory overlapping Azure was accepted")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "azctx")); !os.IsNotExist(err) {
		t.Fatalf("default user-cache path was written: %v", err)
	}
}

func TestCorruptCacheRefreshesAndPrivatePermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/latest") {
			http.Redirect(w, r, "/owner/repo/releases/tag/"+latestTag, http.StatusFound)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	checker := testChecker(t, server, &output)
	dir := filepath.Join(checker.CacheDir, "azctx")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "update.json")
	if err := os.WriteFile(name, []byte(`{"corrupt":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check("owner", "repo"); err != nil || !strings.Contains(output.String(), latestTag) {
		t.Fatalf("corrupt-cache refresh: %v, %q", err, output.String())
	}
	data, err := os.ReadFile(name)
	if err != nil || !strings.Contains(string(data), `"tag":"`+latestTag+`"`) {
		t.Fatalf("cache=%s error=%v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("staging file leaked: %v, %v", entries, err)
	}
	assertPrivateCachePermissions(t, dir, name)
}

func TestUpdateCacheDiagnosticsVerboseOnly(t *testing.T) {
	for _, tc := range []struct {
		name             string
		corrupt, verbose bool
	}{
		{name: "corrupt quiet", corrupt: true},
		{name: "corrupt verbose", corrupt: true, verbose: true},
		{name: "missing quiet"},
		{name: "missing verbose", verbose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/latest") {
					requests.Add(1)
					http.Redirect(w, r, "/owner/repo/releases/tag/"+latestTag, http.StatusFound)
				}
			}))
			defer server.Close()
			var output, diagnostics bytes.Buffer
			checker := testChecker(t, server, &output)
			checker.Current = stableVersion
			log.InitLogger(&diagnostics)
			log.SetVerbose(tc.verbose)
			t.Cleanup(func() {
				log.InitLogger(os.Stderr)
				log.SetVerbose(false)
			})
			if tc.corrupt {
				name := filepath.Join(checker.CacheDir, "azctx", "update.json")
				if err := storage.Write(name, checker.AzureDir, []byte(`{"corrupt":`)); err != nil {
					t.Fatal(err)
				}
			}
			if err := checker.Check("owner", "repo"); err != nil {
				t.Fatalf("cache recovery: %v", err)
			}
			if tc.corrupt && tc.verbose {
				if !strings.Contains(diagnostics.String(), "decode update cache: unexpected end of JSON input") {
					t.Fatalf("missing malformed-cache cause: %q", diagnostics.String())
				}
			} else if diagnostics.Len() != 0 {
				t.Fatalf("unexpected cache diagnostic: %q", diagnostics.String())
			}
			diagnostics.Reset()
			if err := checker.Check("owner", "repo"); err != nil {
				t.Fatalf("recovered-cache check: %v", err)
			}
			if requests.Load() != 1 || output.Len() != 0 || diagnostics.Len() != 0 {
				t.Fatalf("requests=%d output=%q diagnostics=%q", requests.Load(), output.String(), diagnostics.String())
			}
		})
	}
}

func assertPrivateCachePermissions(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	for path, mode := range map[string]os.FileMode{dir: 0o700, name: 0o600, name + ".lock": 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("cache permissions: path=%s info=%v error=%v", path, info, err)
		}
	}
}
