package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/whiteducksoftware/azctx/internal/config"
)

const (
	aliasActionList = "list"
	aliasActionSet  = "set"
)

func newConfigCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: "config [path]", Short: "Print effective configuration or its path",
		Args: cobra.MaximumNArgs(1),
		RunE: commandHandler(func(command *cobra.Command, args []string) error {
			if err := validateOutput(outputFormat(command), "config", formatText); err != nil {
				return err
			}
			if err := command.Context().Err(); err != nil {
				return err
			}
			if len(args) == 1 {
				if args[0] != "path" {
					return errors.New("config accepts only 'path' or no argument")
				}
				path, err := config.Path(deps.ConfigDir)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(command.OutOrStdout(), path)
				return err
			}
			value, err := config.Load(deps.ConfigDir)
			if err != nil {
				return err
			}
			return yaml.NewEncoder(command.OutOrStdout()).Encode(value)
		}),
	}
}

func newAliasCommand(deps Dependencies) *cobra.Command {
	parent := &cobra.Command{Use: "alias", Short: "Manage subscription aliases", Args: cobra.NoArgs}
	for _, action := range []string{aliasActionSet, "rm", aliasActionList} {
		count, use := 0, aliasActionList
		switch action {
		case aliasActionSet:
			count, use = 2, "set NAME SUBSCRIPTION_ID"
		case "rm":
			count, use = 1, "rm NAME"
		}
		parent.AddCommand(&cobra.Command{
			Use: use, Short: "Alias " + action, Args: cobra.ExactArgs(count),
			RunE: commandHandler(func(command *cobra.Command, args []string) error {
				if err := validateOutput(outputFormat(command), "alias "+action, formatText); err != nil {
					return err
				}
				if err := command.Context().Err(); err != nil {
					return err
				}
				value, err := config.Load(deps.ConfigDir)
				if err != nil {
					return err
				}
				if action == aliasActionList {
					return printAliases(command, value.Aliases)
				}
				found := false
				for name := range value.Aliases {
					if strings.EqualFold(name, args[0]) {
						delete(value.Aliases, name)
						found = true
					}
				}
				if action == aliasActionSet {
					value.Aliases[args[0]] = args[1]
				} else if !found {
					return fmt.Errorf("alias %q does not exist", args[0])
				}
				return config.Save(deps.ConfigDir, value)
			}),
		})
	}
	return parent
}

func printAliases(command *cobra.Command, aliases map[string]string) error {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	slices.Sort(names)
	output := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(output, "NAME\tSUBSCRIPTION ID"); err != nil {
		return err
	}
	for _, name := range names {
		if _, err := fmt.Fprintf(output, "%s\t%s\n", name, aliases[name]); err != nil {
			return err
		}
	}
	return output.Flush()
}
