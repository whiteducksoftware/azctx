package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/history"
	"github.com/whiteducksoftware/azctx/internal/profile"
)

func newHistoryCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: "history", Short: "Show the last 100 completed subscription switches", Args: cobra.NoArgs,
		RunE: commandHandler(func(command *cobra.Command, _ []string) error {
			if err := validateOutput(outputFormat(command), "history", formatText, formatJSON); err != nil {
				return err
			}
			if err := command.Context().Err(); err != nil {
				return err
			}
			value, err := history.Load(deps.ConfigDir)
			if errors.Is(err, history.ErrCorrupt) {
				_, _ = fmt.Fprintln(command.ErrOrStderr(), "Warning: corrupt history ignored.")
			} else if err != nil {
				return err
			}
			if outputFormat(command) == formatJSON {
				return json.NewEncoder(command.OutOrStdout()).Encode(value)
			}
			output := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(output, "TIMESTAMP\tPREVIOUS\tNEW\tTENANT\tCLOUD\tSOURCE"); err != nil {
				return err
			}
			for _, entry := range value.Entries {
				previous := "-"
				if entry.Previous != nil {
					previous = entry.Previous.ID
				}
				if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%s\t%s\n", entry.Timestamp.Format(time.RFC3339Nano), previous, entry.New.ID, entry.New.TenantID, entry.New.Environment, entry.Source); err != nil {
					return err
				}
			}
			return output.Flush()
		}),
	}
}

func historyIdentity(sub profile.Subscription) history.Identity {
	return history.Identity{ID: sub.Id, TenantID: sub.Tenant, Environment: sub.Environment}
}

func recordHistory(command *cobra.Command, cli azcli.CLI, deps Dependencies, target profile.Subscription) {
	entry := history.Entry{Timestamp: time.Now().UTC(), New: historyIdentity(target), Source: "script"}
	if current, err := cli.ActiveSubscription(); err == nil {
		previous := historyIdentity(current)
		entry.Previous = &previous
	}
	if stdinIsTerminal(command, deps) {
		entry.Source = "tty"
	}
	warning, err := history.Record(cli.ConfigDir(), entry)
	if warning != nil {
		_, _ = fmt.Fprintln(command.ErrOrStderr(), "Warning: corrupt history ignored.")
	}
	if err != nil {
		_, _ = fmt.Fprintf(command.ErrOrStderr(), "Warning: switch completed but history could not be saved: %v\n", err)
	}
}
