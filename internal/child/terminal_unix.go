//go:build !windows

package child

import (
	"errors"
	"io"
	"os"
	"runtime"
)

// ForkExec blocks SIGTTOU during the handoff. Once azctx is in the background,
// protect both restoration and parent-side terminal output (including TOSTOP).
// Per-thread masking preserves process signal dispositions and subscriptions.
func terminalAccess(action func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	restore, err := blockTerminalStop()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, restore()) }()
	return action()
}

func terminalOutput(output io.Writer) io.Writer {
	if _, attached := output.(*os.File); attached || output == nil {
		return output
	}
	return terminalWriter{output}
}

type terminalWriter struct{ io.Writer }

func (w terminalWriter) Write(data []byte) (n int, err error) {
	err = terminalAccess(func() error {
		n, err = w.Writer.Write(data)
		return err
	})
	return n, err
}
