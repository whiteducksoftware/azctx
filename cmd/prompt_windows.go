package cmd

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	promptKernel           = windows.NewLazySystemDLL("kernel32.dll")
	readConsoleInputPrompt = promptKernel.NewProc("ReadConsoleInputW")
	peekPromptPipe         = promptKernel.NewProc("PeekNamedPipe")
)

func readPromptFile(ctx context.Context, input *os.File, data []byte) error {
	handle := windows.Handle(input.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) == nil {
		return readPromptConsole(ctx, handle, data, mode)
	}
	kind, err := windows.GetFileType(handle)
	if err != nil {
		return err
	}
	for kind == windows.FILE_TYPE_PIPE {
		var available uint32
		ok, _, peekErr := peekPromptPipe.Call(uintptr(handle), 0, 0, 0, uintptr(unsafe.Pointer(&available)), 0)
		if ok == 0 {
			if errors.Is(peekErr, windows.ERROR_BROKEN_PIPE) {
				return io.EOF
			}
			return peekErr
		}
		if available != 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = io.ReadFull(input, data)
	return err
}

func readPromptConsole(ctx context.Context, handle windows.Handle, data []byte, mode uint32) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := windows.WaitForSingleObject(handle, 50)
		if err != nil {
			return err
		}
		if status == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		// INPUT_RECORD is a WORD, padding, then a 16-byte event union. Read
		// one event, not a cooked line, so neither cancellation nor following
		// input for az login can be trapped inside ReadConsole.
		var record [20]byte
		var count uint32
		ok, _, readErr := readConsoleInputPrompt.Call(uintptr(handle), uintptr(unsafe.Pointer(&record[0])), 1, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return readErr
		}
		if count == 0 || binary.LittleEndian.Uint16(record[:2]) != 1 || binary.LittleEndian.Uint32(record[4:8]) == 0 {
			continue
		}
		char := binary.LittleEndian.Uint16(record[14:16])
		if char == 0 {
			continue
		}
		if mode&windows.ENABLE_ECHO_INPUT != 0 {
			if err := echoPromptCharacter(char); err != nil {
				return err
			}
		}
		if char == '\r' {
			char = '\n'
		}
		if char > 127 {
			char = '?' // Confirmations accept only ASCII yes/no.
		}
		data[0] = byte(char)
		return nil
	}
}

func echoPromptCharacter(char uint16) error {
	path, err := windows.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return err
	}
	output, err := windows.CreateFile(path, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	text := []uint16{char}
	if char == '\r' {
		text = append(text, '\n')
	} else if char == '\b' {
		text = append(text, ' ', '\b')
	}
	var written uint32
	err = windows.WriteConsole(output, &text[0], uint32(len(text)), &written, nil) //nolint:gosec // At most three UTF-16 code units.
	return errors.Join(err, windows.CloseHandle(output))
}
