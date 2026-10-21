package azcli

import (
	"errors"
	"slices"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/match"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

const tenantLevelAccountName = "N/A(tenant level account)"

func (cli CLI) Subscriptions() []profile.Subscription {
	return slices.DeleteFunc(slices.Clone(cli.profile.Subscriptions), func(s profile.Subscription) bool {
		return cli.FilterTenantLevelAccount && strings.EqualFold(s.Name, tenantLevelAccountName)
	})
}

func (cli CLI) ActiveSubscription() (profile.Subscription, error) {
	for _, subscription := range cli.profile.Subscriptions {
		if subscription.IsDefault {
			return subscription, nil
		}
	}
	return profile.Subscription{}, errors.New("no active subscription found")
}

func (cli CLI) TryFindSubscription(name string) ([]profile.Subscription, error) {
	return match.Find(cli.Subscriptions(), name)
}
