package cmd_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const consolePromptChildEnv = "AZCTX_TEST_CONSOLE_PROMPT_CHILD"

func TestWindowsConsolePromptCtrlCIs130(t *testing.T) {
	if os.Getenv(consolePromptChildEnv) != "" {
		runConsolePrompt(t)
		return
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, path, "-test.run=^TestWindowsConsolePromptCtrlCIs130$")
	process.Env = append(os.Environ(), consolePromptChildEnv+"=1")
	// This private console prevents the synthetic Ctrl+C reaching the developer
	// or CI shell. No Azure process is started.
	process.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
	if output, err := process.CombinedOutput(); err != nil || ctx.Err() != nil {
		t.Fatalf("native console prompt: err=%v context=%v output=%s", err, ctx.Err(), output)
	}
}

type consoleInterruptOutput struct {
	once sync.Once
	sent chan error
}

func (w *consoleInterruptOutput) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "[y/N]") {
		w.once.Do(func() {
			go func() {
				time.Sleep(100 * time.Millisecond)
				w.sent <- windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0)
			}()
		})
	}
	return len(data), nil
}

func runConsolePrompt(t *testing.T) {
	t.Helper()
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	fake := &testutil.FakeRunner{}
	deps := commandDependencies(t, fake)
	writeSelectionConfig(t, "protected:\n  - "+productionID+"\n")
	output := &consoleInterruptOutput{sent: make(chan error, 1)}
	if code := cmd.Run([]string{productionID}, deps, input, io.Discard, output); code != 130 || len(fake.Calls) != 0 {
		t.Fatalf("console prompt code=%d calls=%+v", code, fake.Calls)
	}
	if err := <-output.sent; err != nil {
		t.Fatal(err)
	}
}
