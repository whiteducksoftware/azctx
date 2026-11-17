// Package match implements subscription search without losing subscription IDs.
package match

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

// Find matches names, tenant names and IDs, preserving source order and identity.
func Find(subscriptions []profile.Subscription, query string) ([]profile.Subscription, error) {
	names, tenants, ids := make([]string, len(subscriptions)), make([]string, len(subscriptions)), make([]string, len(subscriptions))
	for i, subscription := range subscriptions {
		if strings.EqualFold(subscription.Id, query) {
			return []profile.Subscription{subscription}, nil
		}
		names[i] = strings.ToLower(subscription.Name)
		tenants[i] = strings.ToLower(subscription.TenantName)
		ids[i] = strings.ToLower(subscription.Id)
	}
	matched := make([]bool, len(subscriptions))
	for _, field := range [][]string{names, tenants, ids} {
		for _, result := range fuzzy.FindNoSort(strings.ToLower(query), field) {
			matched[result.Index] = true
		}
	}
	var found []profile.Subscription
	seen := make(map[string]bool)
	for i, subscription := range subscriptions {
		id := strings.ToLower(subscription.Id)
		if matched[i] && !seen[id] {
			found = append(found, subscription)
			seen[id] = true
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no azure subscription found for '%s'", query)
	}
	return found, nil
}
