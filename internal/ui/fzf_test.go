package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestFZFExpectedKeyProtocol(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("fzf")), "github.com/whiteducksoftware/azctx/internal/testutil/fakefzf")
	t.Setenv("PATH", dir)
	t.Setenv("FAKEFZF_RECORD", filepath.Join(dir, "record.json"))
	subscriptions := []profile.Subscription{{Id: productionID, Name: "FZF protocol subscription", Tenant: pickerTenantID, TenantName: acmeTenant}}
	row := "0\tFZF protocol subscription\t" + productionID + "\tAcme\t" + pickerTenantID
	for _, tc := range []struct {
		name, output, exit string
		wantIndex          int
		wantErr            error
	}{
		{"Enter", "\n" + row + "\n", "", 0, nil},
		{"Escape", "esc\n", "", -1, ui.ErrCanceled},
		{"Ctrl+C", "", "130", -1, ui.ErrInterrupted},
		{"no match", "", "1", -1, ui.ErrCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FAKEFZF_OUTPUT", tc.output)
			t.Setenv("FAKEFZF_EXIT", tc.exit)
			index, err := ui.SelectFZF(context.Background(), subscriptions, &bytes.Buffer{})
			if index != tc.wantIndex || !errors.Is(err, tc.wantErr) {
				t.Fatalf("selection = %d, %v; want %d, %v", index, err, tc.wantIndex, tc.wantErr)
			}
		})
	}
}

func TestFZFProcessBoundary(t *testing.T) {
	dir := testutil.WorkDir(t)
	testutil.Build(t, filepath.Join(dir, testutil.ExecutableName("fzf")), "github.com/whiteducksoftware/azctx/internal/testutil/fakefzf")
	t.Setenv("PATH", dir)
	record := filepath.Join(dir, "record.json")
	t.Setenv("FAKEFZF_RECORD", record)
	t.Setenv("FAKEFZF_INDEX", "1")
	t.Setenv("FAKEFZF_EXIT", "")
	t.Setenv("FZF_DEFAULT_OPTS", "--bind=start:execute(touch SHOULD_NOT_EXIST)")
	t.Setenv("FZF_DEFAULT_OPTS_FILE", filepath.Join(dir, "untrusted-options"))
	subscriptions := []profile.Subscription{
		{Id: productionID, Name: "--query=$(touch SHOULD_NOT_EXIST)\n0\tforged\x1b[31m", Tenant: pickerTenantID, TenantName: "Tenant\rName\x1b[0m"},
		{Id: productionID, Name: "Same ID, distinct tenant", Tenant: pickerHomeID},
	}
	var messages bytes.Buffer
	index, err := ui.SelectFZF(context.Background(), subscriptions, &messages)
	if err != nil || index != 1 || messages.Len() != 0 {
		t.Fatalf("selection=%d err=%v stderr=%q", index, err, &messages)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args  []string `json:"args"`
		Input string   `json:"input"`
		Opts  string   `json:"opts"`
		File  string   `json:"file"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--no-multi", "--no-sort", "--delimiter=\t", "--with-nth=2..", "--no-ansi", "--expect=esc", "--prompt=azctx> ", "--color=bw"}
	wantInput := "0\t--query=$(touch SHOULD_NOT_EXIST) 0 forged [31m\t" + productionID + "\tTenant Name [0m\t" + pickerTenantID +
		"\n1\tSame ID, distinct tenant\t" + productionID + "\t" + pickerHomeID + "\t" + pickerHomeID + "\n"
	if !reflect.DeepEqual(got.Args, wantArgs) || got.Opts != "" || got.File != "" ||
		got.Input != wantInput || strings.Count(got.Input, "\n") != 2 || strings.ContainsRune(got.Input, '\x1b') ||
		!strings.Contains(got.Input, subscriptions[1].Tenant) || !strings.HasPrefix(got.Input, "0\t") {
		t.Fatalf("unsafe fzf boundary: %+v", got)
	}
	assertFZFCancellation(t, subscriptions)
}

func assertFZFCancellation(t *testing.T, subscriptions []profile.Subscription) {
	t.Helper()
	var messages bytes.Buffer
	for _, tc := range []struct {
		code string
		want error
	}{{"1", ui.ErrCanceled}, {"130", ui.ErrInterrupted}} {
		t.Setenv("FAKEFZF_EXIT", tc.code)
		if _, err := ui.SelectFZF(context.Background(), subscriptions, &messages); !errors.Is(err, tc.want) {
			t.Fatalf("exit %s: %v", tc.code, err)
		}
	}
	t.Setenv("FAKEFZF_EXIT", "")
	row := "1\tSame ID, distinct tenant\t" + productionID + "\t" + pickerHomeID + "\t" + pickerHomeID
	for _, output := range []string{
		"", "\n", row + "\n", "\n1\tforged\n", "tab\n" + row + "\n",
		"\n0" + row[1:] + "\n", "\n" + row + "\n" + row + "\n", "\n" + row + "\n\n",
	} {
		t.Setenv("FAKEFZF_OUTPUT", output)
		if index, err := ui.SelectFZF(context.Background(), subscriptions, &messages); index != -1 || err == nil || err.Error() != "invalid fzf selection" {
			t.Fatalf("invalid output %q: selection=%d err=%v", output, index, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ui.SelectFZF(ctx, subscriptions, &messages); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
}
