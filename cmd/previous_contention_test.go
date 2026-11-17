package cmd_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/history"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/state"
)

const predecessorEnv = "AZCTX_TEST_PREVIOUS_PREDECESSOR"

func TestPreviousPickerRevalidatesFreshProtectedIdentity(t *testing.T) {
	const changedTenant = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	runner := &profileRunner{hook: func(string) error { return nil }}
	deps, private := contentionDependencies(t, runner)
	runner.profile = filepath.Join(deps.ConfigDir, profileFile)
	cfg := config.Default()
	cfg.UpdateCheck, cfg.Protected = false, []string{changedTenant}
	if err := config.Save(deps.ConfigDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(deps.ConfigDir, developmentID); err != nil {
		t.Fatal(err)
	}
	duplicate := `,{"id":"` + developmentID + `","name":"Duplicate","tenantId":"` + changedTenant + `","environmentName":"AzureCloud"}]}`
	writeFixture(t, runner.profile, strings.TrimSuffix(contentionProfile(productionID), "]}")+duplicate)
	freshProfile := strings.ReplaceAll(contentionProfile(productionID), tenantID, changedTenant)
	deps.Select = func(rows []profile.Subscription) (int, error) {
		if !stateLockAvailable(t, private) {
			t.Error("previous switch held state.lock while picking")
		}
		writeFixture(t, runner.profile, freshProfile)
		return slices.IndexFunc(rows, func(row profile.Subscription) bool { return row.Tenant == tenantID }), nil
	}
	var out, messages bytes.Buffer
	status := cmd.Run([]string{"-"}, deps, strings.NewReader(""), &out, &messages)
	if status != 2 || out.Len() != 0 || !strings.Contains(messages.String(), "no longer available; retry") ||
		len(runner.calls()) != 0 {
		t.Fatalf("fresh protected identity: status=%d stdout=%q stderr=%q calls=%v", status, &out, &messages, runner.calls())
	}
	if previous, err := state.Load(deps.ConfigDir); err != nil || previous != developmentID {
		t.Errorf("refused changed identity altered previous=%q, %v", previous, err)
	}
	if recent, err := state.Recent(deps.ConfigDir); err != nil || len(recent) != 0 {
		t.Errorf("refused changed identity recorded MRU=%v, %v", recent, err)
	}
	if data, err := os.ReadFile(runner.profile); err != nil || string(data) != freshProfile {
		t.Errorf("refused changed identity altered refreshed profile=%s, %v", data, err)
	}
}

func startFailingPreviousPredecessor(ctx context.Context, t *testing.T, cancel context.CancelFunc, azure string) func() {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process := exec.CommandContext(ctx, self, "-test.run=^TestPreviousFailingPredecessorProcess$")
	process.Env = append(os.Environ(), predecessorEnv+"="+azure)
	output := &previousSignalWriter{marker: "predecessor-ready", ready: make(chan struct{})}
	process.Stdout, process.Stderr = output, output
	release, err := process.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = release.Close() })
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	cleanProcess(t, process, cancel)
	select {
	case <-output.ready:
	case <-ctx.Done():
		t.Fatal("predecessor never entered its blocking Azure call")
	}
	return func() {
		if _, err := fmt.Fprintln(release, "fail"); err != nil {
			t.Fatal(err)
		}
		if err := process.Wait(); err != nil {
			t.Fatalf("predecessor helper: %v\n%s", err, &output.buffer)
		}
	}
}

func awaitPreviousLock(ctx context.Context, t *testing.T, probe *lockWaitProbe, done <-chan error) {
	t.Helper()
	select {
	case <-probe.waiting:
		if !probe.lockWait {
			t.Fatal("previous switch waited outside the state lock")
		}
	case err := <-done:
		t.Fatalf("previous switch bypassed blocking predecessor: %v", err)
	case <-ctx.Done():
		t.Fatal("previous switch never contended for state.lock")
	}
}

func awaitPreviousResult(ctx context.Context, t *testing.T, done <-chan error, protected bool) {
	t.Helper()
	select {
	case err := <-done:
		if protected {
			if cmd.ExitCode(err) != 1 || !strings.Contains(err.Error(), "requires confirmation") {
				t.Errorf("protected actual target error=%v; want confirmation refusal", err)
			}
		} else if err != nil {
			t.Errorf("previous switch error=%v", err)
		}
	case <-ctx.Done():
		t.Fatal("previous switch did not finish after rollback")
	}
}

func assertCompletedPreviousSwitch(t *testing.T, deps cmd.Dependencies, runner *profileRunner) {
	t.Helper()
	if calls := runner.calls(); !slices.Equal(calls, []string{developmentID}) {
		t.Errorf("wrong-target success after rollback: Azure calls=%v; want previous %s", calls, developmentID)
	}
	assertContentionState(t, deps, productionID, []string{developmentID})
	log, err := history.Load(deps.ConfigDir)
	if err != nil || len(log.Entries) != 1 || log.Entries[0].Previous == nil ||
		log.Entries[0].Previous.ID != productionID || log.Entries[0].New.ID != developmentID {
		t.Errorf("previous switch history=%+v, %v", log, err)
	}
}

func assertRefusedPreviousSwitch(t *testing.T, deps cmd.Dependencies, runner *profileRunner, private, wantPrevious string) {
	t.Helper()
	if calls := runner.calls(); len(calls) != 0 {
		t.Errorf("refused previous switch authorized Azure calls=%v", calls)
	}
	if previous, err := state.Load(deps.ConfigDir); err != nil || previous != wantPrevious {
		t.Errorf("refused previous switch changed state: %q, %v", previous, err)
	}
	if recent, err := state.Recent(deps.ConfigDir); err != nil || len(recent) != 0 {
		t.Errorf("refused previous switch changed MRU: %v, %v", recent, err)
	}
	if data, err := os.ReadFile(runner.profile); err != nil || string(data) != contentionProfile(productionID) {
		t.Errorf("refused previous switch changed profile: %s, %v", data, err)
	}
	if log, err := history.Load(deps.ConfigDir); err != nil || len(log.Entries) != 0 {
		t.Errorf("refused previous switch recorded history: %+v, %v", log, err)
	}
	if !stateLockAvailable(t, private) {
		t.Error("refused previous switch kept state.lock")
	}
}

type previousSignalWriter struct {
	buffer bytes.Buffer
	marker string
	ready  chan struct{}
	once   sync.Once
}

func (w *previousSignalWriter) Write(data []byte) (int, error) {
	n, err := w.buffer.Write(data)
	if bytes.Contains(w.buffer.Bytes(), []byte(w.marker)) {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

func TestPreviousChangeDuringConfirmationRequiresRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	runner := &profileRunner{hook: func(string) error { return nil }}
	deps, private := contentionDependencies(t, runner)
	deps.StdinTTY = func() bool { return true }
	runner.profile = filepath.Join(deps.ConfigDir, profileFile)
	cfg := config.Default()
	cfg.UpdateCheck, cfg.Protected = false, []string{developmentID, otherSubscriptionID}
	if err := config.Save(deps.ConfigDir, cfg); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(deps.ConfigDir, developmentID); err != nil {
		t.Fatal(err)
	}
	input, answer := io.Pipe()
	defer func() { _ = input.Close() }()
	defer func() { _ = answer.Close() }()
	prompt := &previousSignalWriter{marker: "? [y/N] ", ready: make(chan struct{})}
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{"-"})
	root.SetIn(input)
	root.SetOut(io.Discard)
	root.SetErr(prompt)
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	select {
	case <-prompt.ready:
	case err := <-done:
		t.Fatalf("previous switch never prompted: %v", err)
	case <-ctx.Done():
		t.Fatal("previous switch never prompted")
	}
	if !stateLockAvailable(t, private) {
		t.Fatal("previous switch held state.lock while prompting")
	}
	tx, err := state.Begin(ctx, deps.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Record(otherSubscriptionID)
	tx.Release()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(answer, "y"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if cmd.ExitCode(err) != 1 || !strings.Contains(err.Error(), "previous subscription changed") ||
			!strings.Contains(err.Error(), "retry") {
			t.Errorf("confirmation for stale previous target returned %v; want explicit retry failure", err)
		}
	case <-ctx.Done():
		t.Fatal("previous switch did not finish after confirmation")
	}
	assertRefusedPreviousSwitch(t, deps, runner, private, otherSubscriptionID)
}

// This helper is re-executed so the predecessor owns an independent OS lock.
func TestPreviousFailingPredecessorProcess(t *testing.T) {
	azure := os.Getenv(predecessorEnv)
	if azure == "" {
		t.Skip("subprocess helper")
	}
	runner := &profileRunner{
		profile: filepath.Join(azure, profileFile),
		hook: func(string) error {
			if _, err := fmt.Fprintln(os.Stdout, "predecessor-ready"); err != nil {
				return err
			}
			if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
				return err
			}
			return errors.New("predecessor set failed")
		},
	}
	deps := cmd.Dependencies{
		Runner: runner, FS: os.DirFS(azure), ConfigDir: azure,
		StdinTTY: func() bool { return false },
	}
	err := runSwitch(t.Context(), deps, otherSubscriptionID)
	if cmd.ExitCode(err) != 1 || !strings.Contains(err.Error(), "predecessor set failed") {
		t.Fatalf("predecessor error=%v; want failed Azure call", err)
	}
}

func TestPreviousSwitchAfterFailedPredecessor(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(fmt.Sprintf("protected=%t", protected), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			runner := &profileRunner{hook: func(string) error { return nil }}
			deps, private := contentionDependencies(t, runner)
			deps.StdinTTY = func() bool { return false }
			runner.profile = filepath.Join(deps.ConfigDir, profileFile)
			if err := state.Save(deps.ConfigDir, developmentID); err != nil {
				t.Fatal(err)
			}
			if protected {
				cfg := config.Default()
				cfg.UpdateCheck, cfg.Protected = false, []string{developmentID}
				if err := config.Save(deps.ConfigDir, cfg); err != nil {
					t.Fatal(err)
				}
			}
			failPredecessor := startFailingPreviousPredecessor(ctx, t, cancel, deps.ConfigDir)
			if id, err := state.Load(deps.ConfigDir); err != nil || id != productionID {
				t.Fatalf("predecessor did not prepare speculative previous: %q, %v", id, err)
			}
			probe := &lockWaitProbe{Context: ctx, waiting: make(chan struct{})}
			done := make(chan error, 1)
			go func() { done <- runSwitch(probe, deps, "-") }()
			awaitPreviousLock(ctx, t, probe, done)
			if calls := runner.calls(); len(calls) != 0 {
				t.Fatalf("previous switch called Azure before predecessor rollback: %v", calls)
			}
			failPredecessor()
			awaitPreviousResult(ctx, t, done, protected)
			if protected {
				assertRefusedPreviousSwitch(t, deps, runner, private, developmentID)
			} else {
				assertCompletedPreviousSwitch(t, deps, runner)
			}
			if !stateLockAvailable(t, private) {
				t.Error("previous switch kept state.lock")
			}
		})
	}
}
