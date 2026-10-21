package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/child"
)

func newExecCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: "exec <query> -- <cmd> [args]", Short: "Run a command with a context environment, without switching Azure CLI",
		Args: func(command *cobra.Command, args []string) error {
			if command.ArgsLenAtDash() != 1 || len(args) < 2 || strings.TrimSpace(args[0]) == "" || args[1] == "" {
				return errors.New("exec requires exactly one query before -- and a command after it")
			}
			return nil
		},
		RunE: commandHandler(func(command *cobra.Command, args []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			if err := validateOutput(outputFormat(command), "exec", formatText, formatJSON); err != nil {
				return err
			}
			cli, err := readOnlyCLI(deps)
			if err != nil {
				return err
			}
			target, err := contextTarget(command, cli, args[0])
			if err != nil {
				return err
			}
			if err := validateContextIDs(target); err != nil {
				return err
			}
			status, err := child.Run(command.Context(), args[1], args[2:], contextEnvironment(target), child.Streams{
				Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
			})
			if status != 0 {
				return &ChildExitError{Status: status, Cause: err}
			}
			if err != nil {
				return err
			}
			return nil
		}),
	}
}
