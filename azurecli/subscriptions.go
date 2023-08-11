package azurecli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/whiteducksoftware/azctx/utils"

	"github.com/lithammer/fuzzysearch/fuzzy"
)

const TENANT_LEVEL_ACCOUNT_NAME = "N/A(tenant level account)"

var FilterTenantLevelAccount bool = true // Filter tenant level accounts with no available subscriptions, will be manipulated by the flag --filter-tenant-level in cmd/root.go#L47

// Subscriptions returns all subscriptions
func (cli CLI) Subscriptions() utils.ComparableNamedSlice[Subscription] {
	filter := func(s Subscription) bool {
		return !FilterTenantLevelAccount || !strings.EqualFold(s.Name, TENANT_LEVEL_ACCOUNT_NAME)
	}
	return cli.profile.Subscriptions.Filter(filter)
}

// SubscriptionNames returns the names of the given subscriptions
func (cli CLI) SubscriptionNames() []string {
	return cli.Subscriptions().Names()
}

// SetSubscription sets the default subscription in the azure config file
func (cli *CLI) SetSubscription(subscription Subscription) error {
	// Execute az account set command
	args := []string{"account", "set", "--subscription", subscription.Id}
	_, err := utils.ExecuteCommand(AZ_COMMAND, args...)
	if err != nil {
		return err
	}

	return nil
}

// ActiveSubscription returns the active subscription
func (cli CLI) ActiveSubscription() (Subscription, error) {
	// select the subscription with the is default flag set to true
	for _, subscription := range cli.profile.Subscriptions { // Do not use cli.Subscriptions() here, because we want to return the subscription even if it is a tenant level account
		if subscription.IsDefault {
			return subscription, nil
		}
	}

	return Subscription{}, errors.New("no active subscription found")
}

// GetSubscriptionByName returns the azure subscription with the given name
func (cli CLI) GetSubscriptionByName(subscriptionName string) (Subscription, bool) {
	// Find the subscription with the given name
	for _, subscription := range cli.Subscriptions() {
		if strings.EqualFold(subscription.Name, subscriptionName) {
			return subscription, true
		}
	}

	return Subscription{}, false
}

// TryFindSubscription fuzzy searches for the azure subscription in the given AzureProfilesConfig
func (cli CLI) TryFindSubscription(subscriptionName string) (utils.ComparableNamedSlice[Subscription], error) {
	// Fuzzy search for the subscription name
	subscriptionNames := utils.StringSlice(cli.SubscriptionNames())
	results := fuzzy.FindNormalized(strings.ToLower(subscriptionName), subscriptionNames.ToLower())

	switch len(results) {
	case 0:
		// No results found
		return nil, fmt.Errorf("no azure subscription found for '%s'", subscriptionName)
	case 1:
		// One result found
		s, ok := cli.GetSubscriptionByName(results[0])
		if !ok {
			return nil, fmt.Errorf("no azure subscription found for '%s'", subscriptionName)
		}
		return []Subscription{s}, nil
	default:
		// Multiple results found
		subscriptions := make([]Subscription, 0)
		for _, result := range results {
			if s, ok := cli.GetSubscriptionByName(result); ok {
				subscriptions = append(subscriptions, s)
			}
		}

		return subscriptions, nil
	}
}
