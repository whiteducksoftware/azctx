package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
)

func readOnlyCLI(deps Dependencies) (azcli.CLI, error) {
	return azcli.New(deps.FS, deps.Runner, deps.ConfigDir)
}

func contextTarget(command *cobra.Command, cli azcli.CLI, query string) (profile.Subscription, error) {
	cfg, err := config.Load(cli.ConfigDir())
	if err != nil {
		return profile.Subscription{}, err
	}
	if query == "" {
		return cli.ActiveSubscription()
	}
	all, _ := command.Flags().GetBool("all")
	candidates, err := (selection.Catalog{Subscriptions: cli.Subscriptions(), Config: cfg}).Find(query, all)
	if err != nil {
		return profile.Subscription{}, &ExitError{Code: CodeNoMatch, Message: err.Error(), Status: ExitNoMatch, Cause: err}
	}
	if len(candidates) != 1 {
		return profile.Subscription{}, &ExitError{
			Code: CodeAmbiguousMatch, Message: fmt.Sprintf("multiple subscriptions match %q; use a subscription ID or tenant/subscription", query),
			Status: ExitNoMatch, Candidates: SubscriptionOutputs(candidates),
		}
	}
	return candidates[0], nil
}

func validateContextIDs(target profile.Subscription) error {
	if err := profile.ValidateGUID(target.Id); err != nil {
		return fmt.Errorf("invalid subscription GUID: %w", err)
	}
	if err := profile.ValidateGUID(target.Tenant); err != nil {
		return fmt.Errorf("invalid tenant GUID: %w", err)
	}
	return nil
}
