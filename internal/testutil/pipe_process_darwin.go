package testutil

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func watchPipeProcess(pid int) (wait, release func() error, err error) {
	if pid <= 0 {
		return nil, nil, fmt.Errorf("invalid stderr descendant PID %d", pid)
	}
	queue, err := unix.Kqueue()
	if err != nil {
		return nil, nil, err
	}
	release = func() error { return unix.Close(queue) }
	event := unix.Kevent_t{
		Ident: uint64(pid), Filter: unix.EVFILT_PROC,
		Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT,
	}
	if _, err := unix.Kevent(queue, []unix.Kevent_t{event}, nil, nil); err != nil {
		_ = release()
		return nil, nil, err
	}
	wait = func() error {
		var events [1]unix.Kevent_t
		timeout := unix.NsecToTimespec(pipeFixtureTimeout.Nanoseconds())
		n, err := unix.Kevent(queue, nil, events[:], &timeout)
		if err != nil {
			return err
		}
		if n != 1 || events[0].Fflags&unix.NOTE_EXIT == 0 {
			return fmt.Errorf("stderr descendant %d did not exit", pid)
		}
		return nil
	}
	return wait, release, nil
}
