package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func newTenantCommand(newCLI func() (azcli.CLI, error), deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use: "tenant [query]", Short: "Choose a tenant, then a subscription", Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return tenantRunE(command, args, newCLI, deps)
		},
	}
	command.Flags().Bool("yes", false, "Confirm switching to a protected subscription without prompting")
	return command
}

func tenantRunE(command *cobra.Command, args []string, newCLI func() (azcli.CLI, error), deps Dependencies) error {
	if err := validateRootOutput(command, false, len(args) != 0); err != nil {
		return err
	}
	if err := command.Context().Err(); err != nil {
		return err
	}
	cli, err := newCLI()
	if err != nil {
		return err
	}
	all, _ := command.Flags().GetBool("all")
	if len(args) == 0 && (!inputIsTerminal(command, deps) || outputFormat(command) != "") {
		cli.IncludeAll = all
		return listSubscriptions(command, cli)
	}
	catalog, err := tenantCatalog(cli, deps)
	if err != nil {
		return err
	}
	query := ""
	if len(args) != 0 {
		query = args[0]
	}
	tenants, err := catalog.Tenants(query, all)
	if err != nil || len(tenants) == 0 {
		return &ExitError{Code: CodeNoMatch, Message: fmt.Sprintf("no tenant found for %q", query), Status: ExitNoMatch, Cause: err}
	}
	tenant, err := chooseTenant(command, deps, catalog.Config, tenants)
	if err != nil {
		return err
	}
	target, err := pickSubscription([]string{tenant.ID}, tenant.Subscriptions, rankedSelector(command, deps, catalog))
	if err != nil {
		return err
	}
	cli.Context = command.Context()
	return switchConfigured(command, cli, deps, catalog.Config, target, false)
}

func tenantCatalog(cli azcli.CLI, deps Dependencies) (selection.Catalog, error) {
	cfg, err := config.Load(cli.ConfigDir())
	if err != nil {
		return selection.Catalog{}, err
	}
	recent, err := state.Recent(cli.ConfigDir())
	if err != nil {
		return selection.Catalog{}, err
	}
	subscriptions := cli.Subscriptions()
	if deps.Select == nil {
		ui.SortSubscriptions(subscriptions)
	}
	return selection.Catalog{Subscriptions: subscriptions, Config: cfg, Recent: recent}, nil
}

func chooseTenant(command *cobra.Command, deps Dependencies, cfg config.Config, tenants []selection.Tenant) (selection.Tenant, error) {
	if !inputIsTerminal(command, deps) || outputFormat(command) == formatJSON {
		if len(tenants) > 1 || len(tenants[0].Subscriptions) > 1 {
			return selection.Tenant{}, &ExitError{Code: CodeAmbiguousMatch, Message: "multiple matches; use azctx TENANT_GUID/SUBSCRIPTION_GUID", Status: ExitNoMatch, Candidates: SubscriptionOutputs(tenantCandidates(tenants))}
		}
	}
	if len(tenants) == 1 {
		return tenants[0], nil
	}
	// Picker rows carry the tenant GUID as ID; never use a name as a key.
	rows := make([]profile.Subscription, len(tenants))
	for i, tenant := range tenants {
		rows[i] = profile.Subscription{Id: tenant.ID, Name: tenant.Name, Tenant: tenant.ID, TenantName: tenant.Domain}
	}
	selected, err := configuredPicker(command, deps, cfg, rows, true)
	if err != nil {
		return selection.Tenant{}, err
	}
	if selected < 0 || selected >= len(rows) {
		return selection.Tenant{}, fmt.Errorf("invalid tenant picker selection")
	}
	for _, tenant := range tenants {
		if strings.EqualFold(tenant.ID, rows[selected].Id) {
			return tenant, nil
		}
	}
	return selection.Tenant{}, fmt.Errorf("invalid tenant picker identity")
}

func tenantCandidates(tenants []selection.Tenant) []profile.Subscription {
	var result []profile.Subscription
	for _, tenant := range tenants {
		result = append(result, tenant.Subscriptions...)
	}
	return result
}
