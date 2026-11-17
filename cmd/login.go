package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/log"
)

func newLoginCommand(newCLI func() (azcli.CLI, error)) *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Azure",
		Long: `Login to Azure (wrapped around 'az login')
	Authenticates the CLI instance to Azure and fetches all available tenants and subscriptions.
	All args after -- are directly passed to the 'az login' command.`,
		RunE: commandHandler(func(cmd *cobra.Command, args []string) error {
			return loginRunE(cmd, args, newCLI)
		}),
	}
	loginCmd.Flags().Bool("force-mfa", false, "force individual authentication for each tenant separately (required for tenants which enforce explicit MFA)")
	return loginCmd
}

func loginRunE(cmd *cobra.Command, args []string, newCLI func() (azcli.CLI, error)) error {
	// Ensure that the azure cli is installed
	cli, err := newCLI()
	if err != nil {
		return err
	}

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

	log.Info("Successfully fetched %d tenants and %d subscriptions.", len(cli.Tenants()), len(cli.Subscriptions()))
	return nil
}

func refreshData(cmd *cobra.Command, cli azcli.CLI, extraArgs []string) error {
	// Fetch all available tenants
	err := cli.UpdateTenants()
	if err != nil {
		log.Warn("%s", `
`+
			strings.Repeat("-", 80)+`
Failed fetching available tenants, only tenants which do not require explicit MFA / Individual Authentication will be available.
This may be due to the azure cli being completely logged out or due to a network error.
Subsequent logins should no longer have this issue.

Feel free to open an issue at https://github.com/whiteducksoftware/azctx/issues if this issue persists.
`+strings.Repeat("-", 80))
	}

	// Try to refresh the subscriptions
	switch {
	case cmd.Flags().Changed("force-mfa") && err == nil: // Only force MFA if the user explicitly requested it & we were able to fetch the tenants
		err = cli.IterativeTenantLogin(extraArgs)
	default:
		err = cli.InteractiveLogin(extraArgs)
	}

	if err != nil {
		return err
	}

	return nil
}
