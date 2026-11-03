//go:build !windows && !darwin && !linux

package child

// Foreground terminal ownership is supported on the shipped Unix platforms.
const terminalHandoff = false

func blockTerminalStop() (func() error, error) {
	return func() error { return nil }, nil
}
