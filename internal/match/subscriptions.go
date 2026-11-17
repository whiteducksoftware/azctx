// Package match implements read-only subscription resolution without losing IDs.
package match

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

// Options supplies config aliases and ID priorities; Recent is most-recent-first.
type Options struct {
	Aliases map[string]string
	Pinned  []string
	Recent  []string
}

// Rank stably orders pinned subscriptions (or tenants), then recent IDs.
// Ranking never resolves ambiguity; every matching identity remains a candidate.
func (options Options) Rank(subscriptions []profile.Subscription) {
	pinned := map[string]bool{}
	for _, id := range options.Pinned {
		pinned[strings.ToLower(id)] = true
	}
	recent := map[string]int{}
	for i, id := range options.Recent {
		key := strings.ToLower(id)
		if _, exists := recent[key]; !exists {
			recent[key] = i
		}
	}
	rank := func(s profile.Subscription) (bool, int) {
		index, ok := recent[strings.ToLower(s.Id)]
		if !ok {
			index = len(options.Recent)
		}
		return pinned[strings.ToLower(s.Id)] || pinned[strings.ToLower(s.Tenant)], index
	}
	slices.SortStableFunc(subscriptions, func(a, b profile.Subscription) int {
		ap, ar := rank(a)
		bp, br := rank(b)
		if ap != bp {
			if ap {
				return -1
			}
			return 1
		}
		return cmp.Compare(ar, br)
	})
}

func Find(subscriptions []profile.Subscription, query string) ([]profile.Subscription, error) {
	return FindWithOptions(subscriptions, query, Options{})
}

// FindWithOptions resolves ID, then exact names/aliases, then fuzzy fields.
// A qualified query first scopes by exact tenant ID, display name or domain.
func FindWithOptions(subscriptions []profile.Subscription, query string, options Options) ([]profile.Subscription, error) {
	candidates := subscriptions
	if tenant, sub, qualified := strings.Cut(query, "/"); qualified {
		candidates = nil
		for _, candidate := range subscriptions {
			if tenant != "" && (strings.EqualFold(candidate.Tenant, tenant) || strings.EqualFold(candidate.TenantName, tenant) || strings.EqualFold(candidate.TenantDomain, tenant)) {
				candidates = append(candidates, candidate)
			}
		}
		return find(candidates, sub, query, options, false)
	}
	return FindUnqualified(candidates, query, options)
}

// FindUnqualified applies ID/exact/fuzzy tiers without parsing tenant/subscription.
func FindUnqualified(subscriptions []profile.Subscription, query string, options Options) ([]profile.Subscription, error) {
	return find(subscriptions, query, query, options, true)
}

func find(subscriptions []profile.Subscription, query, original string, options Options, includeTenants bool) ([]profile.Subscription, error) {
	exact := func(field func(profile.Subscription) string, value string) []profile.Subscription {
		var found []profile.Subscription
		for _, sub := range subscriptions {
			if value != "" && strings.EqualFold(field(sub), value) {
				found = append(found, sub)
			}
		}
		return found
	}
	found := exact(func(s profile.Subscription) string { return s.Id }, query)
	if len(found) != 0 {
		return finish(found, original, options)
	}
	found = exact(func(s profile.Subscription) string { return s.Name }, query)
	targets := map[string]bool{}
	for alias, id := range options.Aliases {
		if strings.EqualFold(alias, query) {
			targets[strings.ToLower(id)] = true
		}
	}
	if len(targets) != 0 {
		found = nil
		for _, sub := range subscriptions {
			if strings.EqualFold(sub.Name, query) || targets[strings.ToLower(sub.Id)] {
				found = append(found, sub)
			}
		}
		// An unavailable exact alias must never fuzzy-switch to another ID.
		return finish(found, original, options)
	}
	if len(found) == 0 && query != "" {
		found = fuzzyMatches(subscriptions, query, includeTenants)
	}
	return finish(found, original, options)
}

func fuzzyMatches(subscriptions []profile.Subscription, query string, includeTenants bool) []profile.Subscription {
	matched := make([]bool, len(subscriptions))
	fields := make([][]string, 5)
	for i := range fields {
		fields[i] = make([]string, len(subscriptions))
	}
	for i, sub := range subscriptions {
		fields[0][i], fields[1][i] = strings.ToLower(sub.Name), strings.ToLower(sub.Id)
		if includeTenants {
			fields[2][i], fields[3][i], fields[4][i] = strings.ToLower(sub.TenantName), strings.ToLower(sub.TenantDomain), strings.ToLower(sub.Tenant)
		}
	}
	for _, field := range fields {
		for _, result := range fuzzy.FindNoSort(strings.ToLower(query), field) {
			matched[result.Index] = true
		}
	}
	var found []profile.Subscription
	for i, sub := range subscriptions {
		if matched[i] {
			found = append(found, sub)
		}
	}
	return found
}

func finish(found []profile.Subscription, query string, options Options) ([]profile.Subscription, error) {
	if len(found) == 0 {
		return nil, fmt.Errorf("no azure subscription found for '%s'", query)
	}
	result := make([]profile.Subscription, 0, len(found))
	seen := map[string]bool{}
	for _, sub := range found {
		key := strings.ToLower(sub.Environment + "\x00" + sub.Tenant + "\x00" + sub.Id)
		if !seen[key] {
			result = append(result, sub)
			seen[key] = true
		}
	}
	options.Rank(result)
	return result, nil
}
