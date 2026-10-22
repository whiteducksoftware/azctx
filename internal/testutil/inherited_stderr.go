package testutil

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	pipeFixtureTimeoutMillis = 5000
	pipeFixtureTimeout       = pipeFixtureTimeoutMillis * time.Millisecond
)

type pipeDescendant struct {
	connection net.Conn
	reader     *bufio.Reader
	wait       func() error
	release    func() error
	err        error
}

// InheritedStderr owns the lifetime of an actual orphaned stderr holder.
type InheritedStderr struct {
	listener   net.Listener
	accepted   chan pipeDescendant
	descendant *pipeDescendant
	once       sync.Once
	err        error
}

// NewInheritedStderr registers descendant termination before WorkDir cleanup.
// The fixture cannot exit until this owner closes its private control connection.
func NewInheritedStderr(t *testing.T) *InheritedStderr {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &InheritedStderr{listener: listener, accepted: make(chan pipeDescendant, 1)}
	t.Setenv("AZCTX_TEST_PIPE_ADDRESS", listener.Addr().String())
	t.Cleanup(func() {
		if err := fixture.Stop(); err != nil {
			t.Error(err)
		}
	})
	go func() { fixture.accepted <- acceptPipeDescendant(listener) }()
	return fixture
}

func acceptPipeDescendant(listener net.Listener) pipeDescendant {
	connection, err := listener.Accept()
	if err != nil {
		return pipeDescendant{err: err}
	}
	descendant := pipeDescendant{connection: connection, reader: bufio.NewReader(connection)}
	err = connection.SetDeadline(time.Now().Add(pipeFixtureTimeout))
	var pid int
	if err == nil {
		_, err = fmt.Fscanln(descendant.reader, &pid)
	}
	if err == nil {
		// Pin the process's kernel identity while it is blocked awaiting our ACK.
		descendant.wait, descendant.release, err = watchPipeProcess(pid)
	}
	if err == nil {
		_, err = fmt.Fprintln(connection, "ack")
	}
	descendant.err = err
	return descendant
}

func (f *InheritedStderr) receive() *pipeDescendant {
	if f.descendant == nil {
		descendant := <-f.accepted
		f.descendant = &descendant
	}
	return f.descendant
}

// AssertHeld verifies that the descendant still owns its real inherited stderr.
func (f *InheritedStderr) AssertHeld(t *testing.T) {
	t.Helper()
	if f.descendant == nil {
		select {
		case descendant := <-f.accepted:
			f.descendant = &descendant
		case <-time.After(pipeFixtureTimeout):
			t.Fatal("stderr descendant did not connect")
		}
	}
	descendant := f.descendant
	if descendant.err != nil {
		t.Fatal(descendant.err)
	}
	if err := descendant.connection.SetDeadline(time.Now().Add(pipeFixtureTimeout)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(descendant.connection, "held"); err != nil {
		t.Fatal(err)
	}
	response, err := descendant.reader.ReadString('\n')
	if err != nil || strings.TrimSpace(response) != "held" {
		t.Fatalf("stderr descendant exited before cleanup: response=%q error=%v", response, err)
	}
}

// Stop closes the lifetime gate and waits for kernel-confirmed process exit.
// Windows waits on a retained handle, not a potentially recycled PID.
func (f *InheritedStderr) Stop() error {
	f.once.Do(func() {
		listenerErr := f.listener.Close()
		descendant := f.receive()
		if errors.Is(descendant.err, net.ErrClosed) {
			f.err = listenerErr
			return
		}
		f.err = errors.Join(listenerErr, descendant.err)
		if descendant.connection != nil {
			f.err = errors.Join(f.err, descendant.connection.Close())
		}
		if descendant.wait != nil {
			f.err = errors.Join(f.err, descendant.wait(), descendant.release())
		}
	})
	return f.err
}
