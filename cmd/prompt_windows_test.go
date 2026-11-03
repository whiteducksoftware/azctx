package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/whiteducksoftware/azctx/internal/ui"
)

const consoleEditChildEnv = "AZCTX_TEST_CONSOLE_EDIT_CHILD"

func TestWindowsPromptInputRecordABI(t *testing.T) {
	record := promptInputRecord{}
	if unsafe.Sizeof(record) != 20 || unsafe.Alignof(record) != 4 || unsafe.Offsetof(record.char) != 14 {
		t.Fatalf("INPUT_RECORD ABI: size=%d alignment=%d char offset=%d", unsafe.Sizeof(record), unsafe.Alignof(record), unsafe.Offsetof(record.char))
	}
}

func TestWindowsPromptEditing(t *testing.T) {
	if os.Getenv(consoleEditChildEnv) == "" {
		path, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		process := exec.CommandContext(ctx, path, "-test.run=^TestWindowsPromptEditing$")
		process.Env = append(os.Environ(), consoleEditChildEnv+"=1")
		process.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
		if output, err := process.CombinedOutput(); err != nil || ctx.Err() != nil {
			t.Fatalf("console editing: err=%v context=%v output=%s", err, ctx.Err(), output)
		}
		return
	}
	runConsoleEditing(t)
}

type consoleEditCase struct {
	name, keys, answer, echoed string
	interrupt                  bool
}

func runConsoleEditing(t *testing.T) {
	t.Helper()
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	handle := windows.Handle(input.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		t.Fatal(err)
	}
	// Disabling processed input lets the raw Ctrl+C key reach the reader.
	if err := windows.SetConsoleMode(handle, (mode|windows.ENABLE_ECHO_INPUT|windows.ENABLE_LINE_INPUT)&^windows.ENABLE_PROCESSED_INPUT); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.SetConsoleMode(handle, mode) }()
	for _, tc := range []consoleEditCase{
		{"empty backspace and delete", "\b\x7fn\x7fno\r", "no", "no", false},
		{"raw CtrlC", "\x03", "", "", true},
		{"non ASCII", "\u00e9\r", "?", "?", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertConsoleEditing(t, input, output, tc)
		})
	}
}

func assertConsoleEditing(t *testing.T, input, output *os.File, tc consoleEditCase) {
	t.Helper()
	handle, out := windows.Handle(input.Fd()), windows.Handle(output.Fd())
	if err := windows.FlushConsoleInputBuffer(handle); err != nil {
		t.Fatal(err)
	}
	if err := windows.SetConsoleCursorPosition(out, windows.Coord{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprint(output, strings.Repeat(" ", 128)); err != nil {
		t.Fatal(err)
	}
	if err := windows.SetConsoleCursorPosition(out, windows.Coord{}); err != nil {
		t.Fatal(err)
	}
	const prompt = "confirmation [y/N] "
	if _, err := fmt.Fprint(output, prompt); err != nil {
		t.Fatal(err)
	}
	writePromptKeys(t, handle, tc.keys)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	answer, err := readConfirmation(ctx, input)
	if answer != tc.answer || (tc.interrupt && !errors.Is(err, ui.ErrInterrupted)) || (!tc.interrupt && err != nil) {
		t.Fatalf("answer=%q error=%v", answer, err)
	}
	text := readPromptScreen(t, out)
	if got := strings.TrimRight(text, " "); got != strings.TrimRight(prompt+tc.echoed, " ") {
		t.Fatalf("echo erased prompt or wrote raw control/non-ASCII input: %q", text)
	}
	if tc.interrupt {
		assertPromptCursor(t, out, int16(len(prompt)))
	}
}

func assertPromptCursor(t *testing.T, out windows.Handle, column int16) {
	t.Helper()
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(out, &info); err != nil || info.CursorPosition.X != column || info.CursorPosition.Y != 0 {
		t.Fatalf("Ctrl+C moved the echo cursor: info=%+v error=%v", info, err)
	}
}

// The Win32 INPUT_RECORD/KEY_EVENT_RECORD ABI: WORD + padding + a DWORD-aligned
// 16-byte union. This independent input fixture is also used for key injection.
type consoleTestInputRecord struct {
	eventType    uint16
	padding      uint16
	keyDown      int32
	repeat       uint16
	virtualKey   uint16
	scanCode     uint16
	char         uint16
	controlState uint32
}

func writePromptKeys(t *testing.T, handle windows.Handle, keys string) {
	t.Helper()
	var records []consoleTestInputRecord
	for _, char := range utf16.Encode([]rune(keys)) {
		records = append(records, consoleTestInputRecord{eventType: 1, keyDown: 1, repeat: 1, char: char})
	}
	var written uint32
	ok, _, err := promptKernel.NewProc("WriteConsoleInputW").Call(uintptr(handle), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&written)))
	if ok == 0 || written != uint32(len(records)) { //nolint:gosec // Bounded literal test keystrokes.
		t.Fatalf("inject console input: written=%d error=%v", written, err)
	}
}

func readPromptScreen(t *testing.T, handle windows.Handle) string {
	t.Helper()
	var text [128]uint16
	var count uint32
	ok, _, err := promptKernel.NewProc("ReadConsoleOutputCharacterW").Call(uintptr(handle), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), 0, uintptr(unsafe.Pointer(&count)))
	if ok == 0 {
		t.Fatal(err)
	}
	return string(utf16.Decode(text[:count]))
}
