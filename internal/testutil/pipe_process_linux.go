package testutil

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func watchPipeProcess(pid int) (wait, release func() error, err error) {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, nil, err
	}
	if fd < 0 || fd > math.MaxInt32 {
		_ = unix.Close(fd)
		return nil, nil, fmt.Errorf("invalid stderr descendant pidfd %d", fd)
	}
	pollFD := int32(fd)
	wait = func() error {
		events := []unix.PollFd{{Fd: pollFD, Events: unix.POLLIN}}
		n, err := unix.Poll(events, pipeFixtureTimeoutMillis)
		if err != nil {
			return err
		}
		if n != 1 || events[0].Revents&unix.POLLIN == 0 {
			return fmt.Errorf("stderr descendant %d did not exit", pid)
		}
		return nil
	}
	return wait, func() error { return unix.Close(fd) }, nil
}
