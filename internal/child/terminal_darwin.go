package child

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	terminalHandoff = true
	signalBlock     = 1
	signalSetMask   = 3
)

func blockTerminalStop() (func() error, error) {
	mask := uint32(1 << (unix.SIGTTOU - 1))
	var previous uint32
	if err := terminalSignalMask(signalBlock, &mask, &previous); err != nil {
		return nil, err
	}
	return func() error { return terminalSignalMask(signalSetMask, &previous, nil) }, nil
}

func terminalSignalMask(how int, mask, previous *uint32) error {
	// Darwin has no x/sys pthread_sigmask wrapper. This kernel interface uses
	// the same 32-bit sigset_t as libSystem's pthread_sigmask, without cgo.
	_, _, errno := unix.RawSyscall(unix.SYS___PTHREAD_SIGMASK, uintptr(how), uintptr(unsafe.Pointer(mask)), uintptr(unsafe.Pointer(previous))) //nolint:staticcheck // x/sys has no libSystem wrapper for this thread-local syscall.
	if errno != 0 {
		return errno
	}
	return nil
}
