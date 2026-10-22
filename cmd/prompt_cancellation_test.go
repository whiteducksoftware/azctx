package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

type promptOutput struct {
	bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func TestClosedPromptInputFailsWithoutMutation(t *testing.T) {
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	writeSelectionConfig(t, "protected:\n  - "+productionID+"\n")
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := cmd.NewRootCommand(deps)
	command.SetArgs([]string{productionID})
	command.SetIn(input)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)
	go func() { done <- command.ExecuteContext(ctx) }()
	select {
	case err := <-done:
		if cmd.ExitCode(err) != 1 || !errors.Is(err, os.ErrClosed) || len(fake.Calls) != 0 {
			t.Fatalf("closed prompt input: err=%v calls=%+v", err, fake.Calls)
		}
	case <-time.After(time.Second):
		cancel()
		_ = <-done
		t.Fatal("closed prompt input blocked instead of reporting the read failure")
	}
}

func (w *promptOutput) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if strings.Contains(w.String(), "[y/N]") || strings.Contains(w.String(), "[Y/n]") {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

func TestPromptsHonorCancellationWithoutClosingStdin(t *testing.T) {
	for _, prompt := range []string{"protected", "broker", tenantSubcommand} {
		t.Run(prompt, func(t *testing.T) {
			fake := &testutil.FakeRunner{Responses: []testutil.Response{
				{Result: azcli.Result{Stdout: brokerVersionJSON}},
				{Result: azcli.Result{Stderr: mfaFixture(t, "50076")}},
			}}
			deps := commandDependencies(t, fake)
			deps.Platform = platformWindows
			args, calls := []string{productionID}, 0
			switch prompt {
			case "protected":
				writeSelectionConfig(t, "protected:\n  - "+productionID+"\n")
			case "broker":
				args, calls = []string{authCommand, brokerCommand, brokerEnable}, 1
			case tenantSubcommand:
				args, calls = []string{loginCommand}, 2
			}
			input, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			defer writer.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			command := cmd.NewRootCommand(deps)
			command.SetArgs(args)
			command.SetIn(input)
			var stdout bytes.Buffer
			stderr := &promptOutput{ready: make(chan struct{})}
			command.SetOut(&stdout)
			command.SetErr(stderr)
			done := make(chan error, 1)
			go func() { done <- command.ExecuteContext(ctx) }()
			select {
			case <-stderr.ready:
			case <-time.After(3 * time.Second):
				_ = writer.Close()
				_ = <-done
				t.Fatal("confirmation did not appear")
			}
			cancel()
			select {
			case err := <-done:
				if cmd.ExitCode(err) != 130 || !errors.Is(err, context.Canceled) || len(fake.Calls) != calls || stdout.Len() != 0 {
					t.Fatalf("prompt interruption: err=%v calls=%+v stdout=%q stderr=%q", err, fake.Calls, &stdout, stderr)
				}
				if _, err := input.Stat(); err != nil {
					t.Fatalf("cancellation closed caller-owned stdin: %v", err)
				}
			case <-time.After(time.Second):
				_ = writer.Close()
				_ = <-done
				t.Fatal("confirmation read ignored cancellation")
			}
		})
	}
}
