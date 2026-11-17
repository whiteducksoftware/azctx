package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/project"
	"github.com/whiteducksoftware/azctx/internal/selection"
)

const (
	contextMismatchMessage = "active context does not match the required subscription or tenant"
	guardSubscriptionKey   = "subscription"
)

func newGuardCommand(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use: "guard", Short: "Check the active context against flags or the nearest .azctx.yaml",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			if err := validateOutput(outputFormat(command), "guard", formatText, formatJSON); err != nil {
				return err
			}
			cli, err := readOnlyCLI(deps)
			if err != nil {
				return err
			}
			expected, err := guardExpectation(command, cli)
			if err != nil {
				return err
			}
			active, err := cli.ActiveSubscription()
			if err != nil {
				return err
			}
			if err := validateContextIDs(active); err != nil {
				return err
			}
			if err := command.Context().Err(); err != nil {
				return err
			}
			if !expected.Matches(active.Id, active.Tenant) {
				return &ExitError{Code: CodeContextMismatch, Message: contextMismatchMessage, Status: ExitContextDiffers}
			}
			return nil
		},
	}
	command.Flags().String(guardSubscriptionKey, "", "Expected subscription query or GUID")
	command.Flags().String("tenant", "", "Expected tenant query or GUID")
	return command
}

func guardExpectation(command *cobra.Command, cli azcli.CLI) (project.Context, error) {
	if !command.Flags().Changed(guardSubscriptionKey) && !command.Flags().Changed("tenant") {
		value, _, err := project.Find("")
		return value, err
	}
	expected := project.Context{}
	for _, key := range []string{guardSubscriptionKey, "tenant"} {
		if !command.Flags().Changed(key) {
			continue
		}
		query, _ := command.Flags().GetString(key)
		if strings.TrimSpace(query) == "" {
			return project.Context{}, fmt.Errorf("--%s must not be empty", key)
		}
		id, err := guardQuery(command, cli, key, query)
		if err != nil {
			return project.Context{}, err
		}
		if key == guardSubscriptionKey {
			expected.Subscription = id
		} else {
			expected.Tenant = id
		}
	}
	return expected, nil
}

func guardQuery(command *cobra.Command, cli azcli.CLI, key, query string) (string, error) {
	if profile.ValidateGUID(query) == nil {
		return query, nil
	}
	if key == guardSubscriptionKey {
		target, err := contextTarget(command, cli, query)
		return target.Id, err
	}
	cfg, err := config.Load(cli.ConfigDir())
	if err != nil {
		return "", err
	}
	all, _ := command.Flags().GetBool("all")
	tenants, err := (selection.Catalog{Subscriptions: cli.Subscriptions(), Config: cfg}).Tenants(query, all)
	if err != nil {
		return "", &ExitError{Code: CodeNoMatch, Message: err.Error(), Status: ExitNoMatch, Cause: err}
	}
	if len(tenants) == 1 {
		return tenants[0].ID, nil
	}
	var candidates []profile.Subscription
	for _, tenant := range tenants {
		candidates = append(candidates, tenant.Subscriptions...)
	}
	return "", &ExitError{
		Code: CodeAmbiguousMatch, Message: fmt.Sprintf("multiple tenants match %q; use a tenant GUID", query),
		Status: ExitNoMatch, Candidates: SubscriptionOutputs(candidates),
	}
}

func warnProjectContext(command *cobra.Command, subscription profile.Subscription) {
	if format := outputFormat(command); format != "" && format != formatText {
		return
	}
	expected, path, err := project.Find("")
	switch {
	case errors.Is(err, project.ErrNotFound):
		return
	case err != nil:
		_, _ = fmt.Fprintf(command.ErrOrStderr(), "Warning: project context: %s\n", err)
	case !expected.Matches(subscription.Id, subscription.Tenant):
		_, _ = fmt.Fprintf(command.ErrOrStderr(), "Warning: active context differs from %s; run `azctx guard` before deploying.\n", path)
	}
}
