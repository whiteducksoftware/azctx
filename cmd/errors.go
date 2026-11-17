package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/ui"
)

// Exit statuses are the public process boundary, including reserved later-layer statuses.
const (
	ExitSuccess        = 0
	ExitFailure        = 1
	ExitNoMatch        = 2
	ExitSignInFailed   = 3
	ExitContextDiffers = 4
	ExitInterrupted    = 130
)

// JSON error codes are stable identifiers, independent of human message text.
const (
	CodeError           = "error"
	CodeNoMatch         = "no_match"
	CodeAmbiguousMatch  = "ambiguous_match"
	CodeInterrupted     = "interrupted"
	CodeSignInFailed    = "sign_in_failed"
	CodeContextMismatch = "context_mismatch"
)

// ExitError carries a classified failure without exiting inside a command.
// Status 3 and 4 are reserved for the sign-in and guard layers.
type ExitError struct {
	Code       string
	Message    string
	Status     int
	Candidates []SubscriptionOutput
	Cause      error
}

func (e *ExitError) Error() string { return e.Message }
func (e *ExitError) Unwrap() error { return e.Cause }

// ExitCode is the single mapping from errors to process statuses.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ui.ErrInterrupted) {
		return ExitInterrupted
	}
	var classified *ExitError
	if errors.As(err, &classified) {
		switch classified.Status {
		case ExitFailure, ExitNoMatch, ExitSignInFailed, ExitContextDiffers, ExitInterrupted:
			return classified.Status
		}
	}
	return ExitFailure
}

// WriteError presents exactly one JSON object, or a human diagnostic and candidates.
// The return value is suitable for os.Exit; nil errors produce no output.
func WriteError(output io.Writer, format string, err error) int {
	status := ExitCode(err)
	if err == nil {
		return status
	}
	payload := struct {
		Code       string               `json:"code"`
		Message    string               `json:"message"`
		ExitCode   int                  `json:"exit_code"` //nolint:tagliatelle // The public contract names this field exit_code.
		Candidates []SubscriptionOutput `json:"candidates,omitempty"`
	}{Code: CodeError, Message: err.Error(), ExitCode: status}
	var classified *ExitError
	if errors.As(err, &classified) {
		payload.Code, payload.Candidates = classified.Code, classified.Candidates
	}
	if errors.Is(err, ui.ErrInterrupted) {
		payload.Code = CodeInterrupted
	}
	if format == formatJSON {
		_ = json.NewEncoder(output).Encode(payload)
	} else if status != ExitInterrupted {
		_, _ = fmt.Fprintf(output, "Error: %s\n", payload.Message)
		for _, candidate := range payload.Candidates {
			_, _ = fmt.Fprintf(output, "  %s/%s (%s)\n", candidate.TenantID, candidate.Name, candidate.ID)
		}
	}
	return status
}

// HandlerError preserves handler causes without exiting inside a command.
type HandlerError struct {
	err error
}

func (e *HandlerError) Error() string { return e.err.Error() }
func (e *HandlerError) Unwrap() error { return e.err }

func commandHandler(handler func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		if err := handler(command, args); err != nil {
			return &HandlerError{err: err}
		}
		return nil
	}
}
