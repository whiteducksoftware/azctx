package cmd

import (
	"context"
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
		return readPromptConsole(ctx, handle, data)
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

// INPUT_RECORD's event union has DWORD alignment, including on Windows ARM64.
type promptInputRecord struct {
	eventType    uint16
	padding      uint16
	keyDown      int32
	repeat       uint16
	virtualKey   uint16
	scanCode     uint16
	char         uint16
	controlState uint32
}

func readPromptConsole(ctx context.Context, handle windows.Handle, data []byte) error {
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
		var record promptInputRecord
		var count uint32
		ok, _, readErr := readConsoleInputPrompt.Call(uintptr(handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return readErr
		}
		if count == 0 || record.eventType != 1 || record.keyDown == 0 {
			continue
		}
		char := record.char
		if char == 0 {
			continue
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

func echoPromptByte(input io.Reader, char byte) error {
	file, ok := input.(*os.File)
	if !ok {
		return nil
	}
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(file.Fd()), &mode) == nil && mode&windows.ENABLE_ECHO_INPUT != 0 {
		if char == '\n' {
			char = '\r'
		}
		return echoPromptCharacter(uint16(char))
	}
	return nil
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
