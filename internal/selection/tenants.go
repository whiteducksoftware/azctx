package selection

import (
	"strings"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

// Tenant groups subscriptions by token tenant GUID, never by a display label.
type Tenant struct {
	ID, Name, Domain string
	Subscriptions    []profile.Subscription
}

// Tenants is read-only and preserves ranked subscription order within groups.
func (catalog Catalog) Tenants(query string, all bool) ([]Tenant, error) {
	subscriptions := catalog.Visible(all)
	catalog.Options().Rank(subscriptions)
	tenants := make([]Tenant, 0)
	indices := map[string]int{}
	seen := map[string]bool{}
	for _, sub := range subscriptions {
		if err := profile.ValidateGUID(sub.Tenant); err != nil {
			continue
		}
		key := strings.ToLower(sub.Tenant)
		identity := strings.ToLower(sub.Environment + "\x00" + sub.Tenant + "\x00" + sub.Id)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		index, exists := indices[key]
		if !exists {
			index = len(tenants)
			indices[key] = index
			tenants = append(tenants, Tenant{ID: sub.Tenant, Name: sub.TenantLabel(), Domain: sub.TenantDomain})
		}
		tenants[index].Subscriptions = append(tenants[index].Subscriptions, sub)
	}
	if query == "" {
		return tenants, nil
	}
	// Reuse the same ID/exact/fuzzy tiers without subscription aliases.
	projections := make([]profile.Subscription, len(tenants))
	for i, tenant := range tenants {
		name := tenant.Name
		if strings.EqualFold(tenant.Domain, query) {
			// Exact names and domains share the matcher's exact-name tier.
			name = tenant.Domain
		}
		projections[i] = profile.Subscription{Id: tenant.ID, Name: name, Tenant: tenant.ID, TenantName: tenant.Name, TenantDomain: tenant.Domain}
	}
	found, err := match.FindUnqualified(projections, query, match.Options{})
	if err != nil {
		return nil, err
	}
	result := make([]Tenant, 0, len(found))
	for _, projection := range found {
		result = append(result, tenants[indices[strings.ToLower(projection.Id)]])
	}
	return result, nil
}
