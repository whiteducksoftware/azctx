package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/whiteducksoftware/azctx/internal/redact"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

// Exit statuses are the public process boundary; exec children retain their own status.
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
// Status 3 classifies sign-in failures; status 4 classifies guard mismatches.
type ExitError struct {
	Code       string
	Message    string
	Status     int
	Candidates []SubscriptionOutput
	Cause      error
}

func (e *ExitError) Error() string { return e.Message }
func (e *ExitError) Unwrap() error { return e.Cause }

// ChildExitError preserves an attached exec child's status. Its own streams are
// the diagnostic; WriteError must not append an azctx text or JSON error.
type ChildExitError struct {
	Status int
	Cause  error
}

func (e *ChildExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Status)
}
func (e *ChildExitError) Unwrap() error { return e.Cause }

// ExitCode is the single mapping from errors to process statuses.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ui.ErrInterrupted) || errors.Is(err, context.Canceled) {
		return ExitInterrupted
	}
	var child *ChildExitError
	if errors.As(err, &child) {
		return child.Status
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
	var child *ChildExitError
	if errors.As(err, &child) {
		return status
	}
	payload := struct {
		Code       string               `json:"code"`
		Message    string               `json:"message"`
		ExitCode   int                  `json:"exit_code"` //nolint:tagliatelle // The public contract names this field exit_code.
		Candidates []SubscriptionOutput `json:"candidates,omitempty"`
	}{Code: CodeError, Message: redact.Text(err.Error()), ExitCode: status}
	var classified *ExitError
	if errors.As(err, &classified) {
		payload.Code, payload.Candidates = classified.Code, classified.Candidates
	}
	if status == ExitInterrupted {
		payload.Code = CodeInterrupted
	}
	if format == formatJSON {
		_ = json.NewEncoder(output).Encode(payload)
	} else if status != ExitInterrupted {
		_, _ = fmt.Fprintf(output, "Error: %s\n", ui.DisplayText(payload.Message))
		for _, candidate := range payload.Candidates {
			_, _ = fmt.Fprintf(output, "  %s/%s (%s)\n", ui.DisplayText(candidate.TenantID), ui.DisplayText(candidate.Name), ui.DisplayText(candidate.ID))
		}
	}
	return status
}
