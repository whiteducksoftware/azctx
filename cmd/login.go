package cmd

import (
	"github.com/whiteducksoftware/azctx/azurecli"
	"github.com/whiteducksoftware/azctx/log"
	"github.com/whiteducksoftware/azctx/utils"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Azure",
	Long: `Login to Azure (wrapped around 'az login')
	Authenticates the CLI instance to Azure and fetches all available tenants and subscriptions.
	All args after -- are directly passed to the 'az login' command.`,
	Run: utils.WrapCobraCommandHandler(loginRunE),
}

func init() {
	loginCmd.Flags().Bool("force-mfa", false, "force individual authentication for each tenant separately (required for tenants which enforce explicit MFA)")
	rootCmd.AddCommand(loginCmd)
}

func loginRunE(cmd *cobra.Command, args []string) error {
	// Ensure that the azure cli is installed
	cli, err := azurecli.New(afero.NewOsFs())
	if err != nil {
		return err
	}

	// Refresh the subscriptions and tenants
	err = refreshSubscriptions(cmd, cli, args)
	if err != nil {
		return err
	}

	// Refresh the cli instance
	err = cli.Refresh()
	if err != nil {
		return err
	}

	log.Info("Successfully fetched %d tenants and %d subscriptions.", len(cli.Tenants()), len(cli.Subscriptions()))
	return nil
}

func refreshSubscriptions(cmd *cobra.Command, cli azurecli.CLI, extraArgs []string) error {
	var err error

	// Try to refresh the subscriptions
	switch {
	case cmd.Flags().Changed("force-mfa"):
		err = cli.IterativeTenantLogin(extraArgs)
	default:
		err = cli.InteractiveLogin(extraArgs)
	}

	if err != nil {
		return err
	}

	// Fetch all available tenants
	err = cli.UpdateTenants()
	if err != nil {
		return err
	}

	return nil
}
