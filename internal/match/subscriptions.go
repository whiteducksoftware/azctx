// Package match implements the existing subscription-name search.
package match

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

// Find retains legacy matching, including first-match resolution of duplicate
// names. Duplicate-name selection changes belong to the bug-fix layer.
func Find(subscriptions []profile.Subscription, name string) ([]profile.Subscription, error) {
	names := make([]string, len(subscriptions))
	for i, subscription := range subscriptions {
		names[i] = strings.ToLower(subscription.Name)
	}
	results := fuzzy.FindNoSort(strings.ToLower(name), names)
	if len(results) == 0 {
		return nil, fmt.Errorf("no azure subscription found for '%s'", name)
	}
	var found []profile.Subscription
	for _, result := range results {
		for _, subscription := range subscriptions {
			if strings.EqualFold(subscription.Name, result.Str) {
				found = append(found, subscription)
				break
			}
		}
	}
	return found, nil
}
