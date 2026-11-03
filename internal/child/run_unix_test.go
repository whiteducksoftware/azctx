//go:build !windows

package child_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/whiteducksoftware/azctx/internal/child"
)

func TestRunInterruptAfterSuccessfulChild(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	status, err := child.Run(context.Background(), path, []string{"-test.run=^TestSuccessfulChildHelper$"},
		[]string{"AZCTX_TEST_SUCCESSFUL_CHILD=1"}, child.Streams{Stdout: interruptAfterExit{}, Stderr: io.Discard})
	if status != 130 || !errors.Is(err, context.Canceled) {
		t.Fatalf("successful child plus independent interrupt: status=%d error=%v", status, err)
	}
}

func TestSuccessfulChildHelper(_ *testing.T) {
	if os.Getenv("AZCTX_TEST_SUCCESSFUL_CHILD") == "" {
		return
	}
	fmt.Fprintln(os.Stdout, os.Getppid())
	os.Exit(0)
}

type interruptAfterExit struct{}

func (w interruptAfterExit) Write(data []byte) (int, error) {
	// Wait has reaped the successful supervisor, but is still draining this
	// stdout pipe. An interrupt here must not become a command-start error.
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	deadline := time.Now().Add(3 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			return 0, errors.New("successful supervisor did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return 0, err
	}
	if err := process.Signal(os.Interrupt); err != nil {
		return 0, err
	}
	// Keep pipe draining open while the registered forwarding goroutine runs.
	time.Sleep(100 * time.Millisecond)
	return len(data), nil
}
