package azurecli

import (
	"strings"

	"github.com/whiteducksoftware/azctx/utils"

	"github.com/spf13/afero"
)

// CLI represents the azure cli
type CLI struct {
	fs      afero.Fs
	profile Profile
	tenants []Tenant
}

// Profile represents the AzureProfiles.json file
type Profile struct {
	Subscriptions  utils.ComparableNamedSlice[Subscription] `json:"subscriptions"`
	InstallationId string                                   `json:"installationId"`
}

// Tenant represents the response of the azure management api
type Tenant struct {
	Id   string `json:"tenantId"`
	Name string `json:"displayName"`
}

// Subscription represents a subscription in the AzureProfiles.json file
type Subscription struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	User  struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"user"`
	IsDefault    bool   `json:"isDefault"`
	Tenant       string `json:"tenantId"`
	TenantName   string `` // This is a duplicate of Tenant, used only for display purposes
	Environment  string `json:"environmentName"`
	HomeTenantId string `json:"homeTenantId"`
	ManagedBy    []struct {
		Id string `json:"tenantId"`
	} `json:"managedByTenants"`
}

// Implement utils.Compartable interface
func (s Subscription) Compare(other utils.Comparable) int {
	// Check if other is a subscription
	otherSubscription, ok := other.(Subscription)
	if !ok {
		panic("other is not a Subscription")
	}

	// Sort subscriptions by tenant name and then by subscription name
	if strings.EqualFold(s.TenantName, otherSubscription.TenantName) {
		return strings.Compare(s.Name, otherSubscription.Name)
	}

	return strings.Compare(s.TenantName, otherSubscription.TenantName)
}

// Implement utils.Named interface
func (s Subscription) Named() string {
	return s.Name
}
