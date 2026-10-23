//go:build !windows

package child

import (
	"os"
	"syscall"
)

// ExitCode maps process exits and signals to the CLI status convention.
func ExitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}
