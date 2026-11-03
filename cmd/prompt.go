package cmd

import (
	"context"
	"io"
	"os"
)

func readPromptByte(ctx context.Context, input io.Reader, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if file, ok := input.(*os.File); ok {
		if file.Fd() == ^uintptr(0) {
			return os.ErrClosed
		}
		return readPromptFile(ctx, file, data)
	}
	// Injected streaming readers can unblock their own Read by closing. Files
	// use platform readiness instead so the caller's terminal is never closed.
	if closer, ok := input.(io.Closer); ok {
		finished := make(chan struct{})
		stop := context.AfterFunc(ctx, func() {
			_ = closer.Close()
			close(finished)
		})
		defer func() {
			if !stop() {
				<-finished
			}
		}()
	}
	_, err := io.ReadFull(input, data)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
