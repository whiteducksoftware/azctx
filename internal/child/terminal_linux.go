package child

import "golang.org/x/sys/unix"

const terminalHandoff = true

func blockTerminalStop() (func() error, error) {
	var mask, previous unix.Sigset_t
	mask.Val[0] = 1 << (unix.SIGTTOU - 1)
	if err := unix.PthreadSigmask(unix.SIG_BLOCK, &mask, &previous); err != nil {
		return nil, err
	}
	return func() error { return unix.PthreadSigmask(unix.SIG_SETMASK, &previous, nil) }, nil
}
