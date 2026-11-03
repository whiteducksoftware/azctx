package azcli

import (
	"regexp"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
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

var tenantFailureLine = regexp.MustCompile(`^(?:(?:WARNING|ERROR): )?Authentication failed against tenant ([0-9a-fA-F-]{36}) '(.*)': AADSTS([0-9]+):.*$`)

func parseTenantFailures(stderr string) []TenantFailure {
	var failures []TenantFailure
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		match := tenantFailureLine.FindStringSubmatch(line)
		if match == nil || profile.ValidateGUID(match[1]) != nil {
			continue
		}
		failures = append(failures, TenantFailure{
			ID: match[1], Name: match[2], Code: match[3], Message: line,
		})
	}
	return failures
}
