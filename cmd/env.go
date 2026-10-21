package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	shellBash       = "bash"
	shellZsh        = "zsh"
	shellFish       = "fish"
	shellPowerShell = "powershell"
	formatGitHub    = "github"
	formatDevOps    = "azure-devops"
)

func newEnvCommand(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use: "env [query]", Short: "Export a context without changing the Azure CLI default",
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(command, args); err != nil {
				return err
			}
			if len(args) != 0 && strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("env query must not be empty")
			}
			return nil
		},
		RunE: commandHandler(func(command *cobra.Command, args []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			if err := validateOutput(outputFormat(command), "env", formatText); err != nil {
				return err
			}
			syntax, err := environmentSyntax(command)
			if err != nil {
				return err
			}
			cli, err := readOnlyCLI(deps)
			if err != nil {
				return err
			}
			query := ""
			if len(args) != 0 {
				query = args[0]
			}
			target, err := contextTarget(command, cli, query)
			if err != nil {
				return err
			}
			if err := validateContextIDs(target); err != nil {
				return err
			}
			if err := command.Context().Err(); err != nil {
				return err
			}
			return writeEnvironment(command, target, syntax, cli.ConfigDir())
		}),
	}
	command.Flags().String("shell", shellBash, "Shell syntax: bash, zsh, fish, powershell (default bash)")
	command.Flags().String("format", "", "Pipeline format: github (append GITHUB_ENV), azure-devops")
	return command
}

func environmentSyntax(command *cobra.Command) (string, error) {
	format, _ := command.Flags().GetString("format")
	if command.Flags().Changed("format") {
		if command.Flags().Changed("shell") {
			return "", fmt.Errorf("--shell and --format cannot be combined")
		}
		if format != formatGitHub && format != formatDevOps {
			return "", fmt.Errorf("unsupported env format %q: use github, azure-devops", format)
		}
		return format, nil
	}
	shell, _ := command.Flags().GetString("shell")
	switch shell {
	case shellBash, shellZsh, shellFish, shellPowerShell:
		return shell, nil
	default:
		return "", fmt.Errorf("unsupported shell %q: use bash, zsh, fish, powershell", shell)
	}
}

func writeEnvironment(command *cobra.Command, target profile.Subscription, syntax, azureDir string) error {
	var output strings.Builder
	for _, variable := range contextEnvironment(target) {
		key, value, _ := strings.Cut(variable, "=")
		switch syntax {
		case shellBash, shellZsh:
			fmt.Fprintf(&output, "export %s='%s'\n", key, strings.ReplaceAll(value, "'", "'\\''"))
		case shellFish:
			value = strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(value)
			fmt.Fprintf(&output, "set -gx %s '%s';\n", key, value)
		case shellPowerShell:
			fmt.Fprintf(&output, "$env:%s = '%s'\n", key, strings.ReplaceAll(value, "'", "''"))
		case formatGitHub:
			fmt.Fprintf(&output, "%s=%s\n", key, value)
		case formatDevOps:
			value = strings.NewReplacer("%", "%AZP25", "\r", "%0D", "\n", "%0A").Replace(value)
			fmt.Fprintf(&output, "##vso[task.setvariable variable=%s]%s\n", key, value)
		}
	}
	if syntax == formatGitHub {
		return appendGitHubEnvironment(azureDir, []byte(output.String()))
	}
	_, err := fmt.Fprint(command.OutOrStdout(), output.String())
	return err
}

func contextEnvironment(target profile.Subscription) []string {
	return []string{
		"ARM_SUBSCRIPTION_ID=" + target.Id,
		"ARM_TENANT_ID=" + target.Tenant,
		"AZURE_SUBSCRIPTION_ID=" + target.Id,
		"AZURE_TENANT_ID=" + target.Tenant,
	}
}
