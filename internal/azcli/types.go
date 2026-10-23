package azcli

import (
	"github.com/spf13/afero"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

type CLI struct {
	fs        afero.Fs
	runner    Runner
	configDir string
	profile   profile.Profile
	tenants   []Tenant

	FilterTenantLevelAccount bool
}

// Tenant is the legacy tenant-cache entry, retained until layer 08.
type Tenant struct {
	Id   string `json:"tenantId"`
	Name string `json:"displayName"`
}
