package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/selection"
)

func completeSubscriptions(deps Dependencies) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(command *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		// Construction/reload is read-only. Deliberately never resolve an az runner.
		cli, err := azcli.New(deps.FS, nil, deps.ConfigDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := config.Load(cli.ConfigDir())
		if err != nil {
			return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
		}
		all, _ := command.Flags().GetBool("all")
		subs := (selection.Catalog{Subscriptions: cli.Subscriptions(), Config: cfg}).Visible(all)
		seen := map[string]bool{}
		available := map[string]bool{}
		add := func(value string) {
			if value != "" && !strings.ContainsAny(value, "\t\r\n") && strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) {
				seen[value] = true
			}
		}
		for _, sub := range subs {
			available[strings.ToLower(sub.Id)] = true
			add(sub.Name)
			for _, tenant := range []string{sub.TenantName, sub.TenantDomain, sub.Tenant} {
				if tenant != "" {
					add(tenant + "/" + sub.Name)
				}
			}
		}
		for alias, id := range cfg.Aliases {
			if available[strings.ToLower(id)] {
				add(alias)
			}
		}
		result := make([]string, 0, len(seen))
		for value := range seen {
			result = append(result, value)
		}
		slices.Sort(result)
		return result, cobra.ShellCompDirectiveNoFileComp
	}
}
