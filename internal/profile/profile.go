// Package profile models and reads the Azure CLI's subscription profile.
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
)

type Profile struct {
	Subscriptions  []Subscription `json:"subscriptions"`
	InstallationId string         `json:"installationId"`
}

type User struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ManagedTenant struct {
	Id string `json:"tenantId"`
}

type Subscription struct {
	Id           string          `json:"id"`
	Name         string          `json:"name"`
	State        string          `json:"state"`
	User         User            `json:"user"`
	IsDefault    bool            `json:"isDefault"`
	Tenant       string          `json:"tenantId"`
	TenantName   string          `json:"-"`
	Environment  string          `json:"environmentName"`
	HomeTenantId string          `json:"homeTenantId"`
	ManagedBy    []ManagedTenant `json:"managedByTenants"`
}

// Read reads a profile from an fs-relative name. Unknown and missing JSON
// fields are intentionally accepted, and subscriptions are never deduplicated.
func Read(files fs.FS, name string) (Profile, error) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		return Profile{}, fmt.Errorf("failed to read %s: %w", name, err)
	}
	var result Profile
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &result); err != nil {
		return Profile{}, fmt.Errorf("failed to unmarshal %s: %w", name, err)
	}
	return result, nil
}
