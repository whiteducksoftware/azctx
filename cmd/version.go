package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/version"
)

func newVersionCommand() *cobra.Command {
	format := "text"
	command := &cobra.Command{
		Use:   "version",
		Short: "Print azctx version information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			info := version.Get()
			switch format {
			case "text":
				_, err := fmt.Fprintf(command.OutOrStdout(), "azctx version %s (commit %s, built %s, %s)\n", info.Version, info.Commit, info.BuildDate, info.GoVersion)
				return err
			case "json":
				return json.NewEncoder(command.OutOrStdout()).Encode(info)
			default:
				return fmt.Errorf("unsupported version output format %q: use text or json", format)
			}
		},
	}
	command.Flags().StringVarP(&format, "output", "o", format, "Output format: text or json")
	return command
}
