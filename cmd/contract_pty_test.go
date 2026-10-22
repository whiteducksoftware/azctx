//go:build darwin || linux

package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBinaryTTYPickerCtrlCIs130(t *testing.T) {
	binary, config, record := fakeBinary(t, `[]`)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "full picker", true: "ambiguous picker"}[ambiguous], func(t *testing.T) {
			args := []string{}
			if ambiguous {
				writeFixture(t, filepath.Join(config, profileFile), strings.ReplaceAll(fixtureProfile, "Development", productionName))
				args = append(args, productionName)
			}
			runInterruptedPTY(t, binary, args)
		})
	}
	if calls, err := os.ReadFile(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted picker called az=%s error=%v", calls, err)
	}
}

func runInterruptedPTY(t *testing.T, binary string, args []string) {
	t.Helper()
	master, slave := openContractPTY(t)
	if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 120}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	process.Stdin, process.Stdout, process.Stderr = slave, &stdout, slave
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()
	ready := make(chan struct{}, 1)
	screen := make(chan string, 1)
	go readContractScreen(master, ready, screen)
	select {
	case <-ready:
		_, _ = master.Write([]byte{3})
	case <-ctx.Done():
		_ = process.Wait()
		_ = master.Close()
		t.Fatalf("TTY picker did not render: %s", <-screen)
	}
	err := process.Wait()
	_ = master.Close()
	output := <-screen
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 130 || stdout.Len() != 0 ||
		!strings.Contains(output, "\x1b[?25h") ||
		regexp.MustCompile("\x1b\\[[0-9;]*(?:[34][0-9]|9[0-7]|10[0-7])(?:;[0-9]+)*m").MatchString(output) || strings.Contains(output, "\a") {
		t.Fatalf("err=%v stdout=%q screen=%q", err, stdout.String(), output)
	}
}

func readContractScreen(master *os.File, ready chan<- struct{}, screen chan<- string) {
	var output strings.Builder
	notified := false
	buffer := make([]byte, 4096)
	for {
		n, err := master.Read(buffer)
		output.Write(buffer[:n])
		if !notified && strings.Contains(output.String(), "Name") {
			ready <- struct{}{}
			notified = true
		}
		if err != nil {
			screen <- output.String()
			return
		}
	}
}
