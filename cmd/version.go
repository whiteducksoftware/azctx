package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/update"
	"github.com/whiteducksoftware/azctx/internal/version"
	"github.com/whiteducksoftware/azctx/log"
)

func newVersionCommand(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "version",
		Short: "Print azctx version information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			info := version.Get()
			format := outputFormat(command)
			if format == "" {
				format = formatText
			}
			switch format {
			case "text":
				_, err := fmt.Fprintf(command.OutOrStdout(), "azctx version %s (commit %s, built %s, %s)\n", info.Version, info.Commit, info.BuildDate, info.GoVersion)
				if err == nil {
					checkForUpdates(command, deps)
				}
				return err
			case "json":
				return json.NewEncoder(command.OutOrStdout()).Encode(info)
			default:
				return fmt.Errorf("unsupported version output format %q: use text or json", format)
			}
		},
	}
	return command
}

func checkForUpdates(command *cobra.Command, deps Dependencies) {
	_, ci := os.LookupEnv("CI")
	if !outputIsTerminal(command, deps) || ci || os.Getenv("AZCTX_NO_UPDATE_CHECK") == "1" {
		return
	}
	_, azureDir, err := azcli.OpenConfig(deps.FS, deps.ConfigDir)
	if err != nil {
		log.Debug("Update check failed: %v", err)
		return
	}
	cfg, err := config.Load(azureDir)
	if err != nil {
		log.Debug("Update check failed: %v", err)
		return
	}
	if !cfg.UpdateCheck {
		return
	}
	check := deps.CheckUpdates
	if check == nil {
		check = (update.Checker{
			AzureDir: azureDir, Output: command.ErrOrStderr(),
			IsTerminal: func() bool { return outputIsTerminal(command, deps) },
		}).CheckFresh
	}
	if err := check(owner, repo); err != nil {
		log.Debug("Update check failed: %v", err)
	}
}
