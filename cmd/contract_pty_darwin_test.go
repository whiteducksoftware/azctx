package cmd_test

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	terminalGetRequest = unix.TIOCGETA
	terminalSetRequest = unix.TIOCSETA
)

func openContractPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = master.Close() })
	for _, request := range []uint{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
		if err := unix.IoctlSetInt(int(master.Fd()), request, 0); err != nil {
			t.Fatal(err)
		}
	}
	var name [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, master.Fd(), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); errno != 0 { //nolint:staticcheck // x/sys has no public libSystem wrapper for the TIOCPTYGNAME buffer ioctl.
		t.Fatal(errno)
	}
	slave, err := os.OpenFile(string(name[:bytesBeforeNull(name[:])]), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}

func bytesBeforeNull(value []byte) int {
	for i, b := range value {
		if b == 0 {
			return i
		}
	}
	return len(value)
}
