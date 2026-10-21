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
	err = runAttached(command)
	if ctx.Err() != nil {
		return 130, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return processStatus(exit.ProcessState), nil
	}
	if err != nil {
		return 0, fmt.Errorf("start command %q: %w", name, err)
	}
	return 0, nil
}

func runAttached(command *exec.Cmd) error {
	configureProcess(command)
	command.WaitDelay = 2 * time.Second
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, forwardedSignals()...)
	defer signal.Stop(signals)
	if err := command.Start(); err != nil {
		return err
	}
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case value := <-signals:
				_ = forwardSignal(command.Process, value)
			case <-done:
				return
			}
		}
	}()
	err := command.Wait()
	close(done)
	<-stopped
	return err
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
