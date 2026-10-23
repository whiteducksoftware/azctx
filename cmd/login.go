package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/log"
)

func newLoginCommand(newCLI func() (azcli.CLI, error), deps Dependencies) *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Login to Azure",
		Long: `Login to Azure (wrapped around 'az login')
	Requires Azure CLI 2.61.0 or newer; tenant metadata comes from its profile.
	Azure CLI's subscription selector is disabled only for this login process.
	All args after -- are directly passed to the 'az login' command.`,
		RunE: commandHandler(func(cmd *cobra.Command, args []string) error {
			return loginRunE(cmd, args, newCLI, deps)
		}),
	}
	loginCmd.Flags().Bool("force-mfa", false, "force individual authentication for each tenant separately (required for tenants which enforce explicit MFA)")
	loginCmd.Flags().Bool("use-device-code", false, "Sign in using a device code")
	loginCmd.Flags().String("tenant", "", "Tenant GUID or fuzzy profile tenant name/domain")
	return loginCmd
}

func loginRunE(cmd *cobra.Command, args []string, newCLI func() (azcli.CLI, error), deps Dependencies) error {
	if err := validateOutput(outputFormat(cmd), "login", formatText); err != nil {
		return err
	}
	// Ensure that the azure cli is installed
	cli, err := newCLI()
	if err != nil {
		return err
	}
	cli.Terminal = &azcli.Terminal{Stdin: cmd.InOrStdin(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
	cli.Context, cli.Platform = cmd.Context(), deps.Platform
	if cmd.Flags().Changed("tenant") {
		for _, arg := range args {
			if arg == "--tenant" || strings.HasPrefix(arg, "--tenant=") || strings.HasPrefix(arg, "-t") {
				return errors.New("--tenant cannot be combined with a passthrough tenant flag")
			}
		}
		tenant, _ := cmd.Flags().GetString("tenant")
		id, err := cli.ResolveTenant(tenant)
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("force-mfa") {
			return errors.New("--tenant cannot be combined with --force-mfa")
		}
		args = append([]string{"--tenant", id}, args...)
	}
	if deviceCode, _ := cmd.Flags().GetBool("use-device-code"); deviceCode {
		args = append([]string{"--use-device-code"}, args...)
	}
	if headless(deps.Platform) && !azcli.UsesDeviceCode(args) {
		log.Info("Headless session detected; consider 'azctx login --use-device-code' if browser or account-picker sign-in is unavailable.")
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

	log.Info("Successfully fetched %d tenants and %d subscriptions.", cli.TenantCount(), len(cli.Subscriptions()))
	return nil
}

func headless(platform string) bool {
	for _, key := range []string{"SSH_CONNECTION", "CODESPACES", "REMOTE_CONTAINERS"} {
		if _, present := os.LookupEnv(key); present {
			return true
		}
	}
	return platform == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
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
