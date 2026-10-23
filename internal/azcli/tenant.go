package azcli

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

// ResolveTenant accepts a GUID on first login; names/domains resolve only from
// distinct token tenants in the active-cloud profile, including tenant accounts.
func (cli CLI) ResolveTenant(query string) (string, error) {
	if profile.ValidateGUID(query) == nil {
		return query, nil
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("tenant must be a GUID or a profile tenant name/domain")
	}
	var matches []string
	seen := make(map[string]bool)
	for _, tenant := range cli.profile.Subscriptions {
		id := strings.ToLower(tenant.Tenant)
		if tenant.Environment != cli.activeCloud || id == "" || seen[id] {
			continue
		}
		fields := []string{tenant.TenantName, tenant.TenantDomain, tenant.Tenant}
		if len(fuzzy.FindNoSort(strings.ToLower(query), lowerFields(fields))) != 0 {
			matches = append(matches, tenant.Tenant)
			seen[id] = true
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no profile tenant matches; use a tenant GUID for first login")
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple profile tenants match; use a tenant GUID")
	}
	if err := profile.ValidateGUID(matches[0]); err != nil {
		return "", fmt.Errorf("invalid profile tenant: %w", err)
	}
	return matches[0], nil
}

func lowerFields(fields []string) []string {
	for i := range fields {
		fields[i] = strings.ToLower(fields[i])
	}
	return fields
}
