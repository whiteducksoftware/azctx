// Package selection provides read-only candidates for switches and future consumers.
package selection

import (
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

// Catalog contains only profile data, configuration and MRU IDs: no runner or IO.
// Callers supply accounts allowed by their cloud/state policy.
type Catalog struct {
	Subscriptions []profile.Subscription
	Config        config.Config
	Recent        []string
}

func (catalog Catalog) Options() match.Options {
	return match.Options{Aliases: catalog.Config.Aliases, Pinned: catalog.Config.Pinned, Recent: catalog.Recent}
}

func (catalog Catalog) Visible(all bool) []profile.Subscription {
	result := make([]profile.Subscription, 0, len(catalog.Subscriptions))
	for _, sub := range catalog.Subscriptions {
		if all || !config.Contains(catalog.Config.Hidden, sub) {
			result = append(result, sub)
		}
	}
	return result
}

// Find returns every identity in the highest matching tier, never auto-selecting.
func (catalog Catalog) Find(query string, all bool) ([]profile.Subscription, error) {
	return match.FindWithOptions(catalog.Visible(all), query, catalog.Options())
}
