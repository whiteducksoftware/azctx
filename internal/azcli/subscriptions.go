package azcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const tenantLevelAccountName = "N/A(tenant level account)"

func (cli CLI) Subscriptions() []profile.Subscription {
	return slices.DeleteFunc(slices.Clone(cli.profile.Subscriptions), func(s profile.Subscription) bool {
		return !cli.IncludeAll && cli.hidden(s)
	})
}

func (cli CLI) hidden(s profile.Subscription) bool {
	return s.Environment != cli.activeCloud || strings.EqualFold(s.State, "Disabled") || strings.EqualFold(s.Name, tenantLevelAccountName)
}

func (cli CLI) ActiveSubscription() (profile.Subscription, error) {
	if cli.activeIndex >= 0 && cli.activeIndex < len(cli.profile.Subscriptions) {
		sub := cli.profile.Subscriptions[cli.activeIndex]
		if cli.IncludeAll || !cli.hidden(sub) {
			return sub, nil
		}
	}
	return profile.Subscription{}, errors.New("no active subscription found")
}

// The profile's active-cloud isDefault is authoritative. Azure CLI remembers
// defaults for other clouds separately, so consult clouds.config only when the
// profile has no default in this cloud. Never infer a default by display name.
func (cli *CLI) resolveDefault(remembered string) error {
	index, count := -1, 0
	for i, sub := range cli.profile.Subscriptions {
		if sub.Environment == cli.activeCloud && sub.IsDefault {
			index, count = i, count+1
		}
	}
	if count == 0 && remembered != "" {
		if err := profile.ValidateGUID(remembered); err != nil {
			return fmt.Errorf("invalid clouds.config subscription for cloud %s: %w", cli.activeCloud, err)
		}
		for i, sub := range cli.profile.Subscriptions {
			if sub.Environment == cli.activeCloud && strings.EqualFold(sub.Id, remembered) {
				index, count = i, count+1
			}
		}
	}
	if count != 1 {
		index = -1
	}
	cli.activeIndex = index
	for i := range cli.profile.Subscriptions {
		cli.profile.Subscriptions[i].IsDefault = i == index
	}
	return nil
}
