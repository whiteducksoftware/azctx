package cmd

import (
	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/log"
)

func newLoginCommand(newCLI func() (azcli.CLI, error)) *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Azure",
		Long: `Login to Azure (wrapped around 'az login')
	Requires Azure CLI 2.61.0 or newer; tenant metadata comes from its profile.
	Azure CLI's subscription selector is disabled only for this login process.
	All args after -- are directly passed to the 'az login' command.`,
		RunE: commandHandler(func(cmd *cobra.Command, args []string) error {
			return loginRunE(cmd, args, newCLI)
		}),
	}
	loginCmd.Flags().Bool("force-mfa", false, "force individual authentication for each tenant separately (required for tenants which enforce explicit MFA)")
	return loginCmd
}

func loginRunE(cmd *cobra.Command, args []string, newCLI func() (azcli.CLI, error)) error {
	if err := validateOutput(outputFormat(cmd), "login", formatText); err != nil {
		return err
	}
	// Ensure that the azure cli is installed
	cli, err := newCLI()
	if err != nil {
		return err
	}
	cli.Terminal = &azcli.Terminal{Stdin: cmd.InOrStdin(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}

	// Refresh the subscriptions and tenants
	err = refreshData(cmd, cli, args)
	if err != nil {
		return err
	}

	// az login will update the subscriptions file, so we need to reload the data
	err = cli.Reload()
	if err != nil {
		return err
	}

	log.Info("Successfully fetched %d tenants and %d subscriptions.", cli.TenantCount(), len(cli.Subscriptions()))
	return nil
}

func refreshData(cmd *cobra.Command, cli azcli.CLI, extraArgs []string) error {
	var err error
	switch {
	case cmd.Flags().Changed("force-mfa") && cli.TenantCount() > 0:
		err = cli.IterativeTenantLogin(extraArgs)
	default:
		err = cli.InteractiveLogin(extraArgs)
	}

	if err != nil {
		return err
	}

	return nil
}
