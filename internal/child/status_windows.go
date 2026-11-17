package child

import "os"

// ExitCode maps process exits and signals to the CLI status convention.
func ExitCode(state *os.ProcessState) int {
	// The Windows default console Ctrl+C handler uses STATUS_CONTROL_C_EXIT.
	if uint32(state.ExitCode()) == 0xC000013A { //nolint:gosec // Compare the native unsigned exit-status bits, not a numeric conversion.
		return 130
	}
	return state.ExitCode()
}
