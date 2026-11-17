package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestContractInputGates(t *testing.T) {
	for _, gate := range []string{"stdin pipe", envCI, gateEmptyCI, gateFlag, gateEnvironment, jsonFormat} {
		for _, ambiguous := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ambiguous=%t", gate, ambiguous), func(t *testing.T) {
				fake := &testutil.FakeRunner{}
				deps := commandDependencies(t, fake)
				deps.Select = func([]profile.Subscription) (int, error) {
					t.Fatal("restricted input opened picker")
					return -1, nil
				}
				deps.CheckUpdates = func(string, string) error {
					t.Fatal("restricted input checked updates")
					return nil
				}
				args := gateArguments(t, gate, &deps)
				if ambiguous {
					deps.FS = fstest.MapFS{profileFile: &fstest.MapFile{Data: []byte(strings.ReplaceAll(fixtureProfile, "Development", productionName))}}
					args = append(args, argumentDelimiter, productionName)
				}
				var stdout, stderr bytes.Buffer
				code := cmd.Run(args, deps, strings.NewReader(""), &stdout, &stderr)
				if ambiguous {
					if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), productionID) || !strings.Contains(stderr.String(), developmentID) {
						t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
					}
				} else if code != 0 || !strings.Contains(stdout.String(), productionName) || stderr.Len() != 0 {
					t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
				}
				if len(fake.Calls) != 0 {
					t.Fatalf("unexpected subprocess=%+v", fake.Calls)
				}
			})
		}
	}
}

func gateArguments(t *testing.T, gate string, deps *cmd.Dependencies) []string {
	t.Helper()
	switch gate {
	case "stdin pipe":
		deps.StdinTTY = func() bool { return false }
	case envCI:
		t.Setenv(envCI, "1")
	case gateEmptyCI:
		t.Setenv(envCI, "")
	case gateFlag:
		return []string{"--no-input"}
	case gateEnvironment:
		t.Setenv("AZCTX_NO_INPUT", "1")
	case jsonFormat:
		return []string{outputFlag, jsonFormat}
	}
	return nil
}

func TestContractExitBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"success", nil, 0},
		{"ordinary", errors.New("failed"), 1},
		{"no match", &cmd.ExitError{Code: "no_match", Message: missingQuery, Status: cmd.ExitNoMatch}, 2},
		{"reserved sign in", &cmd.ExitError{Code: "sign_in_failed", Message: "sign in failed", Status: cmd.ExitSignInFailed}, 3},
		{"reserved guard", &cmd.ExitError{Code: "context_mismatch", Message: "different context", Status: cmd.ExitContextDiffers}, 4},
		{"interrupt", fmt.Errorf("picker: %w", ui.ErrInterrupted), 130},
		{"invalid status", &cmd.ExitError{Code: "error", Message: "invalid", Status: 42}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := cmd.WriteError(&stderr, jsonFormat, tc.err)
			if code != tc.code || cmd.ExitCode(tc.err) != tc.code || tc.err == nil && stderr.Len() != 0 ||
				tc.err != nil && (!strings.Contains(stderr.String(), fmt.Sprintf(`"exit_code":%d`, tc.code)) || strings.Count(stderr.String(), "\n") != 1) {
				t.Fatalf("code=%d stderr=%q", code, stderr.String())
			}
		})
	}
}

func TestContractInterruptedRunRemains130(t *testing.T) {
	for _, format := range []string{"", jsonFormat} {
		fake := &testutil.FakeRunner{}
		deps := commandDependencies(t, fake)
		deps.Select = func([]profile.Subscription) (int, error) { return -1, ui.ErrInterrupted }
		var stdout, stderr bytes.Buffer
		// JSON never opens a picker; exercise its wrapped typed interrupt at the presenter.
		if format == jsonFormat {
			if code := cmd.WriteError(&stderr, format, fmt.Errorf("select: %w", ui.ErrInterrupted)); code != 130 {
				t.Fatalf("code=%d", code)
			}
			continue
		}
		code := cmd.Run(nil, deps, strings.NewReader(""), &stdout, &stderr)
		if code != 130 || stdout.Len() != 0 || stderr.Len() != 0 || len(fake.Calls) != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, stdout.String(), stderr.String(), fake.Calls)
		}
	}
}

func TestCommandPreservesSelectionErrors(t *testing.T) {
	for _, failure := range []error{
		errors.New("selection failed"),
		context.Canceled,
		ui.ErrInterrupted,
		&cmd.ExitError{Code: cmd.CodeNoMatch, Message: missingQuery, Status: cmd.ExitNoMatch},
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			deps.Select = func([]profile.Subscription) (int, error) { return -1, failure }
			root := cmd.NewRootCommand(deps)
			root.SetArgs([]string{})
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			if err := root.Execute(); err != failure || len(fake.Calls) != 0 { //nolint:errorlint // This regression requires exact error identity, not just errors.Is through a wrapper.
				t.Fatalf("error=(%T) %v want original=(%T) %v calls=%+v", err, err, failure, failure, fake.Calls)
			}
		})
	}
}
