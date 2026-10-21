package cmd

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/ui"
	"github.com/whiteducksoftware/azctx/internal/update"
	"github.com/whiteducksoftware/azctx/log"
)

const (
	// Repository Owner
	owner = "whiteducksoftware"

	// Repository Name
	repo = "azctx"
)

// Dependencies supplies the external boundaries used by the command tree.
// Nil fields use production implementations, resolved lazily where needed.
type Dependencies struct {
	Runner       azcli.Runner
	FS           fs.FS // Read-only filesystem rooted at the Azure configuration directory.
	ConfigDir    string
	Select       func([]profile.Subscription) (int, error)
	CheckUpdates func(string, string) error
}

// NewRootCommand constructs an independent command tree, including flag state.
func NewRootCommand(deps Dependencies) *cobra.Command {
	if deps.Select == nil {
		deps.Select = ui.Select
	}
	if deps.CheckUpdates == nil {
		deps.CheckUpdates = update.Check
	}
	filterTenantLevel := true
	newCLI := func() (azcli.CLI, error) {
		if deps.Runner == nil {
			runner, err := azcli.NewRunner()
			if err != nil {
				return azcli.CLI{}, errors.New("azure cli is not installed. please install it and try again. See here: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli")
			}
			deps.Runner = runner
		}
		cli, err := azcli.New(deps.FS, deps.Runner, deps.ConfigDir)
		cli.FilterTenantLevelAccount = filterTenantLevel
		return cli, err
	}
	rootCmd := &cobra.Command{
		Use:   "azctx [- / -- NAME]",
		Short: "azctx is a CLI tool for managing azure cli subscriptions",
		Long: `azctx is a CLI tool for managing azure cli subscriptions.
	It is a helper for the azure cli and provides a simple interface for managing subscriptions.
	Pass a subscription name to select a specific subscription.
	Pass - to switch to the previous subscription.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(command *cobra.Command, _ []string) {
			log.InitLogger(command.ErrOrStderr())
		},
		RunE: commandHandler(func(cmd *cobra.Command, args []string) error {
			return rootRunE(cmd, args, newCLI, deps)
		}),
	}
	rootCmd.AddCommand(newVersionCommand())

	rootCmd.Flags().BoolP("current", "c", false, "Display the current active subscription")
	rootCmd.Flags().BoolP("short", "s", false, `Use a short prompt.
	Deprecated. Size is now automatically determined.`)
	rootCmd.Flags().BoolVar(&filterTenantLevel, "filter-tenant-level", true, "Filter tenant level accounts with no available subscriptions")
	rootCmd.AddCommand(newLoginCommand(newCLI))
	return rootCmd
}

func Execute() error {
	command, err := NewRootCommand(Dependencies{}).ExecuteC()
	// Cobra adds this hint only when command discovery fails, before invocation.
	if err != nil && command != nil && command.CalledAs() == "" {
		return fmt.Errorf("%w\nRun '%s --help' for usage.", err, command.CommandPath())
	}
	return err
}

func rootRunE(cmd *cobra.Command, args []string, newCLI func() (azcli.CLI, error), deps Dependencies) error {
	// Initialize the CLI
	cli, err := newCLI()
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
		return getActiveSubscription(cmd, cli)
	case len(args) == 1:
		// check if the user passed -
		if args[0] == "-" {
			return errors.New("not implemented")
		}

		err = selectSubscriptionByName(cli, args[0], deps.Select)
		if err != nil {
			return err
		}
	default:
		err = interactivelySelectSubscription(cli, deps.Select)
		if err != nil {
			return err
		}
	}

	return deps.CheckUpdates(owner, repo)
}

func interactivelySelectSubscription(cli azcli.CLI, selectItem func([]profile.Subscription) (int, error)) error {
	// Ask the user to select a subscription
	subscriptions := cli.Subscriptions()

	// Run the prompt
	idx, err := selectItem(subscriptions)
	if err != nil {
		if errors.Is(err, ui.ErrInterrupted) {
			return err
		}
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

func selectSubscriptionByName(cli azcli.CLI, name string, selectItem func([]profile.Subscription) (int, error)) error {
	subscriptions, err := cli.TryFindSubscription(name)
	if err != nil {
		return err
	}

	// Check if we found more than one subscription
	var subscription profile.Subscription
	switch length := len(subscriptions); {
	case length > 1:
		// Run the prompt
		idx, err := selectItem(subscriptions)
		if err != nil {
			if errors.Is(err, ui.ErrInterrupted) {
				return err
			}
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

func getActiveSubscription(command *cobra.Command, cli azcli.CLI) error {
	// Try to get the active subscription
	subscription, err := cli.ActiveSubscription()
	if err != nil {
		return err
	}

	// Print the active subscription
	_, err = fmt.Fprintf(command.OutOrStdout(), "Active subscription: %s/%s (%s/%s)\n", subscription.TenantName, subscription.Name, subscription.Tenant, subscription.Id)
	return err
}
