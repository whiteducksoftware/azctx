package azcli

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/redact"
)

// TenantFailure is a documented Azure CLI cross-tenant authentication warning.
type TenantFailure struct {
	ID      string
	Name    string
	Code    string
	Message string
}

// Retryable distinguishes MFA/registration from policy blocks and other errors.
func (failure TenantFailure) Retryable() bool {
	return failure.Code == "50076" || failure.Code == "50079"
}

// LoginResult preserves tenant warnings even when the attached process fails.
type LoginResult struct {
	Attempted bool
	Failures  []TenantFailure
}

var tenantFailureLine = regexp.MustCompile(`^(?:(?:WARNING|ERROR): )?Authentication failed against tenant ([0-9a-fA-F-]{36})(?: '(.*)')?: AADSTS([0-9]+):.*$`)

// ParseTenantFailures classifies transient Azure CLI diagnostics into safe
// metadata. Returned names and messages are redacted, never raw stderr.
func ParseTenantFailures(stderr string) []TenantFailure {
	var failures []TenantFailure
	for _, line := range strings.Split(stderr, "\n") {
		if failure, ok := parseTenantFailure(line); ok {
			failures = append(failures, failure)
		}
	}
	return failures
}

func parseTenantFailure(line string) (TenantFailure, bool) {
	line = strings.TrimSpace(line)
	match := tenantFailureLine.FindStringSubmatch(line)
	if match == nil || profile.ValidateGUID(match[1]) != nil {
		return TenantFailure{}, false
	}
	name := redact.Text(match[2])
	id := strings.Clone(match[1])
	if name == "" {
		name = id
	}
	return TenantFailure{
		ID: id, Name: name, Code: strings.Clone(match[3]), Message: redact.Text(line),
	}, true
}

// tenantFailureWriter observes one raw line transiently, while forwarding every
// write immediately through the diagnostic redactor (including partial prompts).
type tenantFailureWriter struct {
	output  io.Writer
	observe func(TenantFailure)
	pending []byte
}

func (w *tenantFailureWriter) Write(data []byte) (int, error) {
	remaining := data
	for len(remaining) != 0 {
		index := bytes.IndexByte(remaining, '\n')
		if index < 0 {
			w.pending = append(w.pending, remaining...)
			break
		}
		w.pending = append(w.pending, remaining[:index]...)
		w.flush()
		remaining = remaining[index+1:]
	}
	return w.output.Write(data)
}

func (w *tenantFailureWriter) flush() {
	if failure, ok := parseTenantFailure(string(w.pending)); ok {
		w.observe(failure)
	}
	clear(w.pending)
	w.pending = w.pending[:0]
}
