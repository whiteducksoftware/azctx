package history_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/history"
	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func entry() history.Entry {
	return history.Entry{
		Timestamp: time.Date(2026, 10, 21, 22, 0, 0, 0, time.UTC),
		New: history.Identity{
			ID:       "11111111-1111-1111-1111-111111111111",
			TenantID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Environment: "AzureCloud",
		},
		Source: "script",
	}
}

func TestHistoryBoundedPrivateAtomicAndDeterministic(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	azure := filepath.Join(testutil.WorkDir(t), "azure")
	if got, err := history.Load(azure); err != nil || got.SchemaVersion != 1 || got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("missing history=%+v err=%v", got, err)
	}
	for i := range 105 {
		next := entry()
		next.Timestamp = next.Timestamp.Add(time.Duration(i) * time.Second)
		if warning, err := history.Record(azure, next); warning != nil || err != nil {
			t.Fatalf("record %d warning=%v err=%v", i, warning, err)
		}
	}
	got, err := history.Load(azure)
	if err != nil || len(got.Entries) != 100 || got.Entries[0].Timestamp.Format(time.RFC3339) != "2026-10-21T22:01:44Z" ||
		got.Entries[99].Timestamp.Format(time.RFC3339) != "2026-10-21T22:00:05Z" {
		t.Fatalf("cap/order=%+v err=%v", got, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("staging files left behind: %v err=%v", files, err)
	}
	assertPrivateHistoryModes(t, dir)
}

func assertPrivateHistoryModes(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		for _, path := range []string{dir, filepath.Join(dir, "history.json"), filepath.Join(dir, "history.lock")} {
			info, err := os.Stat(path)
			want := os.FileMode(0o600)
			if path == dir {
				want = 0o700
			}
			if err != nil || info.Mode().Perm() != want {
				t.Fatalf("private mode %s: %v %v", path, info, err)
			}
		}
	}
}

func TestHistoryCorruptionIsIgnoredAndRecoveredWithWarning(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	azure := filepath.Join(testutil.WorkDir(t), "azure")
	for _, corrupt := range []string{
		"{secret", "null", `{"schemaVersion":9,"entries":[]}`, `{"schemaVersion":1,"entries":null}`,
		`{"schemaVersion":1,"entries":[]} {}`, `{"schemaVersion":1,"entries":[],"user":"secret"}`,
		`{"schemaVersion":1,"entries":[{"timestamp":"2026-10-21T00:00:00Z","previous":null,"new":{"id":"not-guid","tenantId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","environment":"AzureCloud"},"source":"script"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte(corrupt), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := history.Load(azure)
		if !errors.Is(err, history.ErrCorrupt) || len(got.Entries) != 0 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("corrupt load=%+v err=%v", got, err)
		}
		warning, err := history.Record(azure, entry())
		if !errors.Is(warning, history.ErrCorrupt) || err != nil {
			t.Fatalf("corrupt recovery warning=%v err=%v", warning, err)
		}
		if got, err := history.Load(azure); err != nil || len(got.Entries) != 1 {
			t.Fatalf("recovered history=%+v err=%v", got, err)
		}
	}
}

func TestHistoryRejectsOversizedTrailingData(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	t.Setenv("AZCTX_CONFIG_DIR", dir)
	data := `{"schemaVersion":1,"entries":[]}` + strings.Repeat(" ", 1<<20) + "trailing garbage"
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := history.Load(filepath.Join(dir, "azure")); !errors.Is(err, history.ErrCorrupt) || len(got.Entries) != 0 {
		t.Fatalf("oversized trailing data accepted: %+v err=%v", got, err)
	}
}

func TestHistoryIsolationAndContention(t *testing.T) {
	dir := testutil.IsolateUserDirs(t)
	azure := filepath.Join(dir, "azure")
	if err := os.Mkdir(azure, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZURE_CONFIG_DIR", azure)
	t.Setenv("AZCTX_CONFIG_DIR", azure)
	if _, err := history.Load(""); err == nil {
		t.Fatal("read accepted Azure directory")
	}
	if _, err := history.Record("", entry()); err == nil {
		t.Fatal("write accepted Azure directory")
	}
	owned := filepath.Join(dir, "private")
	t.Setenv("AZCTX_CONFIG_DIR", owned)
	release, acquired, err := storage.Lock(filepath.Join(owned, "history.lock"), azure)
	if err != nil || !acquired {
		t.Fatalf("lock: acquired=%t err=%v", acquired, err)
	}
	if _, err := history.Record(azure, entry()); err == nil {
		t.Fatal("concurrent writer bypassed shared lock")
	}
	release()
	if warning, err := history.Record(azure, entry()); warning != nil || err != nil {
		t.Fatalf("unlocked write: %v %v", warning, err)
	}
	data, err := os.ReadFile(filepath.Join(owned, "history.json"))
	const want = "{\"schemaVersion\":1,\"entries\":[{\"timestamp\":\"2026-10-21T22:00:00Z\",\"previous\":null,\"new\":{\"id\":\"11111111-1111-1111-1111-111111111111\",\"tenantId\":\"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa\",\"environment\":\"AzureCloud\"},\"source\":\"script\"}]}\n"
	if err != nil || string(data) != want || !json.Valid(data) {
		t.Fatalf("canonical history=%q err=%v", data, err)
	}
	files, err := os.ReadDir(azure)
	if err != nil || len(files) != 0 {
		t.Fatalf("Azure modified: %v %v", files, err)
	}
}
