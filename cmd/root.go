package cmd

import (
	"errors"
	"os"

	"github.com/whiteducksoftware/azctx/azurecli"
	"github.com/whiteducksoftware/azctx/log"
	"github.com/whiteducksoftware/azctx/prompt"
	"github.com/whiteducksoftware/azctx/updates"
	"github.com/whiteducksoftware/azctx/utils"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"go.szostok.io/version/extension"
)

const (
	// Respository Owner
	owner = "whiteducksoftware"

	// Repository Name
	repo = "azctx"
)

var rootCmd = &cobra.Command{
	Use:   "azctx [- / -- NAME]",
	Short: "azctx is a CLI tool for managing azure cli subscriptions",
	Long: `azctx is a CLI tool for managing azure cli subscriptions.
	It is a helper for the azure cli and provides a simple interface for managing subscriptions.
	Pass a subscription name to select a specific subscription.
	Pass - to switch to the previous subscription.`,
	SilenceUsage: true,
	Run:          utils.WrapCobraCommandHandler(rootRunE),
}

func init() {
	rootCmd.AddCommand(
		extension.NewVersionCobraCmd(
			extension.WithUpgradeNotice(owner, repo),
		),
	)

	rootCmd.Flags().BoolP("current", "c", false, "Display the current active subscription")
	rootCmd.Flags().BoolP("refresh", "r", false, `Re-Authenticate and refresh the subscriptions. 
	Deprecated. Please use azctx login instead.`)
	rootCmd.Flags().BoolP("short", "s", false, `Use a short prompt.
	Deprecated. Size is now automatically determined.`)
	rootCmd.Flags().BoolVar(&azurecli.FilterTenantLevelAccount, "filter-tenant-level", true, "Filter tenant level accounts with no available subscriptions")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func rootRunE(cmd *cobra.Command, args []string) error {
	// Initialize the CLI
	cli, err := azurecli.New(afero.NewOsFs())
	if err != nil {
		return err
	}

	// check deprecated flags
	if cmd.Flags().Changed("short") {
		log.Warn("Deprecated flag --short/-s used. Size is now automatically determined.")
	}

	// check the flags
	switch {
	case cmd.Flags().Changed("current"):
		return getActiveSubscription(cli)
	case cmd.Flags().Changed("refresh"):
		log.Warn("Deprecated flag --refresh/-r used. Please use `azctx login` instead.")
		return refreshSubscriptions(cmd, cli, args)
	case len(args) == 1:
		// check if the user passed -
		if args[0] == "-" {
			return errors.New("not implemented")
		}

		err = selectSubscriptionByName(cli, args[0])
		if err != nil {
			return err
		}
	default:
		err = interactivelySelectSubscription(cli)
		if err != nil {
			return err
		}
	}

	return updates.Check(owner, repo)
}

func interactivelySelectSubscription(cli azurecli.CLI) error {
	// Ask the user to select a subscription
	subscriptions := cli.Subscriptions()
	prompt := prompt.BuildPrompt(subscriptions)

	// Run the prompt
	idx, _, err := prompt.Run()
	if err != nil {
		return nil
	}

	// Set the selected subscription as the default
	subscription := subscriptions[idx]
	log.Info("Setting active subscription to %s/%s (%s/%s)", subscription.TenantName, subscription.Name, subscription.Tenant, subscription.Id)
	err = cli.SetSubscription(subscription)
	if err != nil {
		return err
	}

	return nil
}

func selectSubscriptionByName(cli azurecli.CLI, name string) error {
	subscriptions, err := cli.TryFindSubscription(name)
	if err != nil {
		return err
	}

	// Check if we found more than one subscription
	var subscription azurecli.Subscription
	switch length := len(subscriptions); {
	case length > 1:
		// Run the prompt
		prompt := prompt.BuildPrompt(subscriptions)
		idx, _, err := prompt.Run()
		if err != nil {
			return nil
		}

		subscription = subscriptions[idx]
	case length == 1:
		subscription = subscriptions[0]
	default:
		return errors.New("no subscription found")
	}

	// Set the selected subscription as the default
	log.Info("Setting active subscription to %s/%s (%s/%s)", subscription.TenantName, subscription.Name, subscription.Tenant, subscription.Id)
	err = cli.SetSubscription(subscription)
	if err != nil {
		return err
	}

	return nil
}

func getActiveSubscription(cli azurecli.CLI) error {
	// Try to get the active subscription
	subscription, err := cli.ActiveSubscription()
	if err != nil {
		return err
	}

	// Print the active subscription
	log.Info("Active subscription: %s/%s (%s/%s)", subscription.TenantName, subscription.Name, subscription.Tenant, subscription.Id)

	return nil
}
