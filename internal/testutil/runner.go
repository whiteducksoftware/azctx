// Package testutil provides hermetic Azure CLI test doubles.
package testutil

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/redact"
)

type Call struct {
	Args     []string `json:"args"`
	Env      []string `json:"env"`
	Attached bool     `json:"attached"`
}

type Response struct {
	Result azcli.Result
	Err    error
}

// FakeRunner records calls and consumes one canned response per invocation.
// Tests should supply responses explicitly: an unexpected call is an error.
type FakeRunner struct {
	Calls     []Call
	Responses []Response
}

func (f *FakeRunner) Run(_ context.Context, args []string, options azcli.Options) (azcli.Result, error) {
	f.Calls = append(f.Calls, Call{
		Args: slices.Clone(args), Env: slices.Clone(options.Env), Attached: options.Terminal != nil,
	})
	if len(f.Responses) == 0 {
		return azcli.Result{ExitCode: -1}, errors.New("unexpected az invocation")
	}
	response := f.Responses[0]
	f.Responses = f.Responses[1:]
	if options.ObserveTenantFailure != nil {
		for _, failure := range azcli.ParseTenantFailures(response.Result.Stderr) {
			options.ObserveTenantFailure(failure)
		}
	}
	response.Result.Stderr = redact.Text(response.Result.Stderr)
	if terminal := options.Terminal; terminal != nil {
		stdout, stderr := terminal.Stdout, terminal.Stderr
		if stdout == nil {
			stdout = os.Stdout
		}
		if stderr == nil {
			stderr = os.Stderr
		}
		if _, err := io.WriteString(stdout, response.Result.Stdout); err != nil {
			return response.Result, err
		}
		if _, err := io.WriteString(stderr, response.Result.Stderr); err != nil {
			return response.Result, err
		}
		response.Result.Stdout = ""
	}
	return response.Result, response.Err
}
