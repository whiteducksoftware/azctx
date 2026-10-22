package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

const (
	// SchemaVersion identifies the subscription output contract.
	SchemaVersion = 1
	formatJSON    = "json"
	formatText    = "text"
	formatTable   = "table"
)

// SubscriptionOutput is shared by list, current and ambiguity candidates.
type SubscriptionOutput struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	TenantID     string       `json:"tenantId"`
	TenantName   string       `json:"tenantName"`
	TenantDomain string       `json:"tenantDomain"`
	State        string       `json:"state"`
	IsDefault    bool         `json:"isDefault"`
	User         profile.User `json:"user"`
	Environment  string       `json:"environment"`
	Delegated    bool         `json:"delegated"`
}

// SubscriptionOutputs preserves profile order and never emits a null list.
func SubscriptionOutputs(subscriptions []profile.Subscription) []SubscriptionOutput {
	result := make([]SubscriptionOutput, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		result = append(result, SubscriptionOutput{
			ID: subscription.Id, Name: subscription.Name, TenantID: subscription.Tenant,
			State: subscription.State, IsDefault: subscription.IsDefault,
			User: subscription.User, Environment: subscription.Environment,
			TenantName: subscription.TenantLabel(), TenantDomain: subscription.TenantDomain,
			Delegated: subscription.Delegated(),
		})
	}
	return result
}

func outputFormat(command *cobra.Command) string {
	format, _ := command.Flags().GetString("output")
	return format
}

func validateOutput(format, context string, allowed ...string) error {
	if format == "" || slices.Contains(allowed, format) {
		return nil
	}
	return fmt.Errorf("unsupported %s output format %q: use %s", context, format, strings.Join(allowed, ", "))
}

func validateRootOutput(command *cobra.Command, current, query bool) error {
	switch {
	case current:
		return validateOutput(outputFormat(command), "current", formatText, formatJSON, "id", "name", "tenant")
	case query:
		return validateOutput(outputFormat(command), "switch", formatText, formatJSON)
	default:
		return validateOutput(outputFormat(command), "list", formatTable, formatJSON)
	}
}

func newListCommand(newCLI func() (azcli.CLI, error)) *cobra.Command {
	command := &cobra.Command{
		Use: "list", Short: "List subscriptions", Args: cobra.NoArgs,
		RunE: commandHandler(func(command *cobra.Command, _ []string) error {
			if err := validateOutput(outputFormat(command), "list", formatTable, formatJSON); err != nil {
				return err
			}
			cli, err := newCLI()
			if err != nil {
				return err
			}
			all, _ := command.Flags().GetBool("all")
			cli.IncludeAll = all
			return listSubscriptions(command, cli)
		}),
	}
	return command
}

func listSubscriptions(command *cobra.Command, cli azcli.CLI) error {
	cfg, err := config.Load(cli.ConfigDir())
	if err != nil {
		return err
	}
	all, _ := command.Flags().GetBool("all")
	visible := (selection.Catalog{Subscriptions: cli.Subscriptions(), Config: cfg}).Visible(all)
	subscriptions := SubscriptionOutputs(visible)
	if outputFormat(command) == formatJSON {
		return json.NewEncoder(command.OutOrStdout()).Encode(struct {
			SchemaVersion int                  `json:"schemaVersion"`
			Subscriptions []SubscriptionOutput `json:"subscriptions"`
		}{SchemaVersion, subscriptions})
	}
	output := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(output, "NAME\tID\tTENANT\tSTATE\tDEFAULT\tACCOUNT\tCLOUD\tACCESS"); err != nil {
		return err
	}
	for _, subscription := range subscriptions {
		model := profile.Subscription{User: subscription.User}
		access := ""
		if subscription.Delegated {
			access = "delegated"
		}
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\n",
			ui.DisplayText(subscription.Name), ui.DisplayText(subscription.ID), ui.DisplayText(subscription.TenantName),
			ui.DisplayText(subscription.State), subscription.IsDefault, ui.DisplayText(model.AccountType()),
			ui.DisplayText(subscription.Environment), access); err != nil {
			return err
		}
	}
	if err := output.Flush(); err != nil {
		return err
	}
	legacyProfileHint(command, visible)
	return nil
}

func legacyProfileHint(command *cobra.Command, subscriptions []profile.Subscription) {
	if format := outputFormat(command); (format == "" || format == formatText || format == formatTable) && profile.HasLegacyTenantMetadata(subscriptions) {
		_, _ = fmt.Fprintln(command.ErrOrStderr(), "Tenant metadata is missing; run `az login` once with Azure CLI 2.61.0 or newer.")
	}
}

func writeCurrent(command *cobra.Command, subscription profile.Subscription, protected bool) error {
	switch outputFormat(command) {
	case formatJSON:
		return json.NewEncoder(command.OutOrStdout()).Encode(struct {
			SchemaVersion int                `json:"schemaVersion"`
			Subscription  SubscriptionOutput `json:"subscription"`
		}{SchemaVersion, SubscriptionOutputs([]profile.Subscription{subscription})[0]})
	case "id":
		_, err := fmt.Fprintln(command.OutOrStdout(), subscription.Id)
		return err
	case "name":
		_, err := fmt.Fprintln(command.OutOrStdout(), subscription.Name)
		return err
	case "tenant":
		_, err := fmt.Fprintln(command.OutOrStdout(), subscription.Tenant)
		return err
	default:
		marker := ""
		if protected {
			marker = " [protected]"
		}
		_, err := fmt.Fprintf(command.OutOrStdout(), "Active subscription: %s/%s (%s/%s)%s\n",
			ui.DisplayText(subscription.TenantLabel()), ui.DisplayText(subscription.DisplayName()),
			ui.DisplayText(subscription.Tenant), ui.DisplayText(subscription.Id), marker)
		return err
	}
}
