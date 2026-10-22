package azcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/whiteducksoftware/azctx/internal/child"
	"github.com/whiteducksoftware/azctx/internal/redact"
)

// Runner is the boundary for all Azure CLI processes. A nonzero process exit
// returns both its captured result and an error; startup failures use code -1.
// Implementations redact returned and attached stderr, classifying transient
// raw lines first when Options.ObserveTenantFailure is set.
type Runner interface {
	Run(context.Context, []string, Options) (Result, error)
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Options struct {
	// Env overrides entries in the inherited process environment.
	Env []string
	// Terminal selects attached mode. Nil selects captured mode.
	Terminal *Terminal
	// ObserveTenantFailure receives classified, redacted login warnings before
	// display redaction can obscure their AADSTS codes. It never receives raw text.
	ObserveTenantFailure func(TenantFailure)
}

// Terminal supplies the attached streams. Nil streams use the process terminal.
// Stderr is always also captured, redacted, in Result, even in attached mode.
type Terminal struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type execRunner struct {
	path string
}

// Path is the executable resolved once by NewRunner, for safe diagnostics.
func (r *execRunner) Path() string { return r.path }

// NewRunner resolves az exactly once, before any invocation.
func NewRunner() (Runner, error) {
	path, err := exec.LookPath("az")
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &execRunner{path: path}, nil
}

func (r *execRunner) Run(ctx context.Context, args []string, options Options) (Result, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, r.path, args...) //nolint:gosec // The executable is az, resolved once by LookPath; arguments are not shell code.
	command.Env = append(os.Environ(), options.Env...)
	var diagnostic io.Writer = &stderr
	command.Stdout = &stdout
	if terminal := options.Terminal; terminal != nil {
		command.Stdin, command.Stdout = terminal.Stdin, terminal.Stdout
		if command.Stdin == nil {
			command.Stdin = os.Stdin
		}
		if command.Stdout == nil {
			command.Stdout = os.Stdout
		}
		terminalErr := terminal.Stderr
		if terminalErr == nil {
			terminalErr = os.Stderr
		}
		diagnostic = io.MultiWriter(&stderr, terminalErr)
	}
	safeStderr := redact.NewWriter(diagnostic)
	command.Stderr = safeStderr
	var failures *tenantFailureWriter
	if options.ObserveTenantFailure != nil {
		failures = &tenantFailureWriter{output: safeStderr, observe: options.ObserveTenantFailure}
		command.Stderr = failures
	}
	err := child.RunAttached(command)
	if failures != nil {
		failures.flush()
	}
	err = errors.Join(err, safeStderr.Flush())
	if ctx.Err() != nil {
		err = errors.Join(ctx.Err(), err)
	}
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: -1}
	if command.ProcessState != nil {
		result.ExitCode = child.ExitCode(command.ProcessState)
	}
	return result, err
}
