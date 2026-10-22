//go:build !windows

package cmd

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func readPromptFile(ctx context.Context, input *os.File, data []byte) error {
	poll := []unix.PollFd{{Fd: int32(input.Fd()), Events: unix.POLLIN}} //nolint:gosec // Native Unix file descriptors are signed 32-bit integers.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := unix.Poll(poll, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return os.NewSyscallError("poll prompt", err)
		}
		if n > 0 {
			_, err := io.ReadFull(input, data)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
}
