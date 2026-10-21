// Package profile models and reads the Azure CLI's subscription profile.
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
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
	TenantName   string          `json:"tenantDisplayName"`
	TenantDomain string          `json:"tenantDefaultDomain"`
	Environment  string          `json:"environmentName"`
	HomeTenantId string          `json:"homeTenantId"`
	ManagedBy    []ManagedTenant `json:"managedByTenants"`
}

// TenantLabel uses the token tenant's ID when older profiles omit its name.
func (s Subscription) TenantLabel() string {
	if s.TenantName != "" {
		return s.TenantName
	}
	return s.Tenant
}

// Delegated identifies a subscription accessed through a different token tenant.
func (s Subscription) Delegated() bool {
	return s.HomeTenantId != "" && s.Tenant != "" && !strings.EqualFold(s.HomeTenantId, s.Tenant)
}

// AccountType distinguishes managed identities before the service-principal type.
func (s Subscription) AccountType() string {
	switch s.User.Name {
	case "systemAssignedIdentity":
		return "system-assigned identity"
	case "userAssignedIdentity":
		return "user-assigned identity"
	}
	switch s.User.Type {
	case "user":
		return "user"
	case "servicePrincipal":
		return "service principal"
	default:
		return s.User.Type
	}
}

// DisplayName labels state, account type and delegation without changing identity.
func (s Subscription) DisplayName() string {
	name := s.Name
	if strings.EqualFold(s.State, "Warned") || strings.EqualFold(s.State, "PastDue") {
		name += " [" + s.State + "]"
	}
	if account := s.AccountType(); account != "" {
		name += " [" + account + "]"
	}
	if s.Delegated() {
		name += " [delegated]"
	}
	return name
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
