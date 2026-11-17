package azcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const accountCommand = "account"

// DiscoverTenants lists account tenants through Azure CLI, not profile metadata
// or a tenant cache. Call after the initial, version-checked user login.
func (cli CLI) DiscoverTenants() ([]string, error) {
	if err := cli.context().Err(); err != nil {
		return nil, err
	}
	result, err := cli.runner.Run(cli.context(), []string{accountCommand, "tenant", "list", "-o", "json"}, Options{
		Env: []string{"AZURE_EXTENSION_USE_DYNAMIC_INSTALL=no"},
	})
	if cli.context().Err() != nil {
		return nil, cli.context().Err()
	}
	if result.ExitCode == 130 {
		return nil, errors.Join(context.Canceled, err)
	}
	if err != nil {
		return nil, fmt.Errorf("az account tenant list failed; ensure that command is available: %w", err)
	}
	var tenants []struct {
		ID string `json:"tenantId"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &tenants); err != nil || tenants == nil {
		return nil, errors.New("az account tenant list returned an invalid tenant array")
	}
	ids := make([]string, 0, len(tenants))
	seen := make(map[string]bool)
	for _, tenant := range tenants {
		if profile.ValidateGUID(tenant.ID) != nil {
			return nil, errors.New("az account tenant list returned an invalid tenant GUID")
		}
		key := strings.ToLower(tenant.ID)
		if !seen[key] {
			ids = append(ids, tenant.ID)
			seen[key] = true
		}
	}
	return ids, nil
}
