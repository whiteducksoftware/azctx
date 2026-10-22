// Package child executes a single attached command without a shell or global
// environment changes. It has no Azure runner, configuration or state access.
package child

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func Run(ctx context.Context, name string, args, env []string, streams Streams) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return 0, fmt.Errorf("resolve command %q: %w", name, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	command := exec.CommandContext(ctx, path, args...)
	command.Args[0] = name
	command.Env = environment(env)
	command.Stdin, command.Stdout, command.Stderr = streams.Stdin, streams.Stdout, streams.Stderr
	err = RunAttached(command)
	if ctx.Err() != nil {
		return 130, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return ExitCode(exit.ProcessState), nil
	}
	if err != nil {
		return 0, fmt.Errorf("start command %q: %w", name, err)
	}
	return 0, nil
}

// RunAttached starts an already-resolved command with signal forwarding and a
// bounded cancellation grace period. The target's resolved path and argv remain
// unchanged, including when a Unix supervisor mediates process-group signals.
func RunAttached(command *exec.Cmd) error {
	hooks, err := configureProcess(command)
	if err != nil {
		return err
	}
	defer hooks.close()
	command.WaitDelay = 2 * time.Second
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, forwardedSignals()...)
	defer signal.Stop(signals)
	control := &signalControl{command: command}
	command.Cancel = func() error { return control.interrupt() }
	if err := command.Start(); err != nil {
		hooks.started()
		return errors.Join(err, hooks.finish())
	}
	hooks.started()
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case value := <-signals:
				control.signal(value)
			case <-done:
				return
			}
		}
	}()
	err = command.Wait()
	// A successful child may leave a browser holding an inherited stderr pipe.
	// Normalize only Wait's pipe timeout, before adding independent errors.
	if errors.Is(err, exec.ErrWaitDelay) && command.ProcessState != nil && command.ProcessState.Success() {
		err = nil
	}
	close(done)
	<-stopped
	interrupted := control.finish()
	if interrupted || (command.ProcessState != nil && ExitCode(command.ProcessState) == 130) {
		_ = killProcess(command.Process)
		err = errors.Join(err, context.Canceled)
	}
	finishErr := hooks.finish()
	if command.ProcessState == nil && finishErr != nil {
		if interrupted {
			return errors.Join(context.Canceled, finishErr)
		}
		return finishErr
	}
	return errors.Join(err, finishErr)
}

type processHooks struct {
	started func()
	finish  func() error
	close   func()
}

type signalControl struct {
	mu          sync.Mutex
	command     *exec.Cmd
	interrupted bool
	count       int
	timer       *time.Timer
}

func (c *signalControl) interrupt() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interruptLocked()
}

func (c *signalControl) interruptLocked() error {
	if c.interrupted {
		return nil
	}
	c.interrupted = true
	c.timer = time.AfterFunc(2*time.Second, func() { _ = killProcess(c.command.Process) })
	if err := forwardSignal(c.command.Process, os.Interrupt); err != nil {
		return killProcess(c.command.Process)
	}
	return nil
}

func (c *signalControl) signal(value os.Signal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	if c.count > 1 {
		c.interrupted = c.interrupted || value == os.Interrupt
		_ = killProcess(c.command.Process)
		return
	}
	if value == os.Interrupt {
		_ = c.interruptLocked()
	} else {
		_ = forwardSignal(c.command.Process, value)
	}
}

func (c *signalControl) finish() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
	}
	return c.interrupted
}

func environment(overrides []string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, inherited := range os.Environ() {
		key, _, _ := strings.Cut(inherited, "=")
		replaced := false
		for _, override := range overrides {
			overrideKey, _, _ := strings.Cut(override, "=")
			if key == overrideKey || (runtime.GOOS == "windows" && strings.EqualFold(key, overrideKey)) {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, inherited)
		}
	}
	return append(result, overrides...)
}
