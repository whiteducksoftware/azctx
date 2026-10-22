package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/history"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

// contentionProfile is a real azureProfile.json whose default is the given ID.
func contentionProfile(active string) string {
	var rows []string
	for _, sub := range []struct{ id, name string }{{productionID, productionName}, {developmentID, developmentName}, {otherSubscriptionID, "Third"}} {
		row := `{"id":"` + sub.id + `","name":"` + sub.name + `","tenantId":"` + tenantID + `","tenantDisplayName":"` + tenantID +
			`","tenantDefaultDomain":"example.invalid","environmentName":"AzureCloud","isDefault":` + strconv.FormatBool(sub.id == active) + `}`
		rows = append(rows, row)
	}
	return `{"subscriptions":[` + strings.Join(rows, ",") + `]}`
}

// profileRunner acts like az account set on a mutable on-disk profile: success
// makes the target the default. Each set first runs the test's hook.
type profileRunner struct {
	profile string
	mu      sync.Mutex
	sets    []string
	hook    func(target string) error
}

func (r *profileRunner) Run(ctx context.Context, args []string, _ azcli.Options) (azcli.Result, error) {
	if len(args) != 4 || !slices.Equal(args[:3], []string{accountCommand, setCommand, subscriptionFlag}) {
		return azcli.Result{ExitCode: -1}, errors.New("unexpected az invocation")
	}
	r.mu.Lock()
	r.sets = append(r.sets, args[3])
	r.mu.Unlock()
	if err := r.hook(args[3]); err != nil {
		return azcli.Result{ExitCode: 1, Stderr: err.Error()}, errors.Join(err, errors.New("exit status 1"))
	}
	if err := os.WriteFile(r.profile, []byte(contentionProfile(args[3])), 0o600); err != nil {
		return azcli.Result{ExitCode: 1}, err
	}
	return azcli.Result{}, ctx.Err()
}

func (r *profileRunner) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.sets)
}

// lockWaitProbe reports the first time the waiting switch blocks on its
// context and records whether that wait is inside storage.WaitLock, which
// proves the second switch found state.lock held and is contending for it.
type lockWaitProbe struct {
	context.Context
	once     sync.Once
	waiting  chan struct{}
	lockWait bool
}

func (p *lockWaitProbe) Done() <-chan struct{} {
	p.once.Do(func() {
		pcs := make([]uintptr, 32)
		frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
		for frame, more := frames.Next(); more; frame, more = frames.Next() {
			p.lockWait = p.lockWait || strings.HasSuffix(frame.Function, "internal/storage.WaitLock")
		}
		close(p.waiting)
	})
	return p.Context.Done()
}

func contentionDependencies(t *testing.T, runner azcli.Runner) (cmd.Dependencies, string) {
	t.Helper()
	deps := commandDependencies(t, &testutil.FakeRunner{})
	private := filepath.Dir(writeSelectionConfig(t, "updateCheck: false\n"))
	azure := testutil.WorkDir(t)
	writeFixture(t, filepath.Join(azure, profileFile), contentionProfile(productionID))
	deps.Runner, deps.FS, deps.ConfigDir, deps.Select = runner, os.DirFS(azure), azure, nil
	return deps, private
}

func runSwitch(ctx context.Context, deps cmd.Dependencies, target string) error {
	root := cmd.NewRootCommand(deps)
	root.SetArgs([]string{target})
	root.SetIn(strings.NewReader(""))
	var out, messages bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&messages)
	if err := root.ExecuteContext(ctx); err != nil {
		return fmt.Errorf("%w (stderr %q)", err, messages.String())
	}
	return nil
}

func TestContendedSwitchRecordsActualPreviousContext(t *testing.T) {
	for _, tc := range []struct {
		name         string
		firstFails   bool
		second       string
		wantPrevious string
		wantRecent   []string
		wantHistory  [][2]string // newest first, as history stores it
	}{
		{
			name: "predecessor commits", second: otherSubscriptionID, wantPrevious: developmentID,
			wantRecent:  []string{otherSubscriptionID, developmentID},
			wantHistory: [][2]string{{developmentID, otherSubscriptionID}, {productionID, developmentID}},
		},
		{
			name: "predecessor commits then return", second: productionID, wantPrevious: developmentID,
			wantRecent:  []string{productionID, developmentID},
			wantHistory: [][2]string{{developmentID, productionID}, {productionID, developmentID}},
		},
		{
			name: "predecessor fails", firstFails: true, second: otherSubscriptionID, wantPrevious: productionID,
			wantRecent:  []string{otherSubscriptionID},
			wantHistory: [][2]string{{productionID, otherSubscriptionID}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inAz, contended := make(chan struct{}), make(chan struct{})
			runner := &profileRunner{}
			runner.hook = func(target string) error {
				if target != developmentID {
					return nil
				}
				// The first switch holds state.lock here until the second waits.
				close(inAz)
				<-contended
				if tc.firstFails {
					return errors.New("set failed")
				}
				return nil
			}
			deps, private := contentionDependencies(t, runner)
			runner.profile = filepath.Join(deps.ConfigDir, profileFile)

			first := make(chan error, 1)
			go func() { first <- runSwitch(context.Background(), deps, developmentID) }()
			<-inAz
			// The second switch snapshots the profile while the first is inside az.
			probe := &lockWaitProbe{Context: context.Background(), waiting: make(chan struct{})}
			second := make(chan error, 1)
			go func() { second <- runSwitch(probe, deps, tc.second) }()
			<-probe.waiting
			if !probe.lockWait {
				t.Fatal("second switch blocked outside the state lock wait")
			}
			select {
			case err := <-second:
				t.Fatalf("second switch bypassed the held state lock: %v", err)
			default:
			}
			if !slices.Equal(runner.calls(), []string{developmentID}) {
				t.Fatalf("az ran before the waiting switch acquired state.lock: %v", runner.calls())
			}
			close(contended)
			if err := <-first; (err != nil) != tc.firstFails {
				t.Fatalf("first switch error = %v", err)
			}
			if err := <-second; err != nil {
				t.Fatalf("second switch error = %v", err)
			}

			assertContentionState(t, deps, tc.wantPrevious, tc.wantRecent)
			log, err := history.Load(deps.ConfigDir)
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]string
			for _, entry := range log.Entries {
				if entry.Previous == nil {
					t.Fatalf("history entry without previous: %+v", entry)
				}
				got = append(got, [2]string{entry.Previous.ID, entry.New.ID})
			}
			if !slices.Equal(got, tc.wantHistory) {
				t.Errorf("history previous/new = %v, want %v", got, tc.wantHistory)
			}
			if !stateLockAvailable(t, private) {
				t.Fatal("switches kept the state lock")
			}
		})
	}
}

func assertContentionState(t *testing.T, deps cmd.Dependencies, wantPrevious string, wantRecent []string) {
	t.Helper()
	previous, err := state.Load(deps.ConfigDir)
	if err != nil || previous != wantPrevious {
		t.Errorf("state previous = %q, %v; want %s", previous, err, wantPrevious)
	}
	if recent, err := state.Recent(deps.ConfigDir); err != nil || !slices.Equal(recent, wantRecent) {
		t.Errorf("state recent = %v, %v; want %v", recent, err, wantRecent)
	}
	if data, err := os.ReadFile(filepath.Join(deps.ConfigDir, profileFile)); err != nil || string(data) != contentionProfile(wantRecent[0]) {
		t.Errorf("active Azure profile = %s, %v; want default %s", data, err, wantRecent[0])
	}
}

func TestSwitchProfileReloadFailureMakesNoChanges(t *testing.T) {
	runner := &profileRunner{hook: func(string) error { return nil }}
	deps, private := contentionDependencies(t, runner)
	profilePath := filepath.Join(deps.ConfigDir, profileFile)
	runner.profile = profilePath
	// The picker runs after the initial profile load and before state.lock.
	deps.Select = func(rows []profile.Subscription) (int, error) {
		writeFixture(t, profilePath, "{")
		for i, row := range rows {
			if row.Id == developmentID {
				return i, nil
			}
		}
		return -1, errors.New("development subscription not offered")
	}
	var out, messages bytes.Buffer
	status := cmd.Run(nil, deps, strings.NewReader(""), &out, &messages)
	if status != 1 || out.Len() != 0 || !strings.Contains(messages.String(), "reload Azure profile before switching (no changes made)") {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, &out, &messages)
	}
	if calls := runner.calls(); len(calls) != 0 {
		t.Fatalf("az ran after a failed profile reload: %v", calls)
	}
	for _, name := range []string{"state.json", "history.json"} {
		if _, err := os.Stat(filepath.Join(private, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s written after a failed profile reload: %v", name, err)
		}
	}
	if !stateLockAvailable(t, private) {
		t.Fatal("failed reload kept the state lock")
	}
}
