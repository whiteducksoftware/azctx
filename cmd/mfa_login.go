package cmd

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

func loginWithMFA(command *cobra.Command, cli azcli.CLI, args []string, deps Dependencies) error {
	result, err := cli.Login(args)
	if ExitCode(err) == ExitInterrupted {
		return err
	}
	if command.Context().Err() != nil {
		return command.Context().Err()
	}
	if !result.Attempted {
		return err
	}
	if azcli.UsesWorkloadIdentity(args) {
		if err != nil {
			return signInFailed("Azure CLI sign-in failed; see its attached output", err)
		}
		return finishLogin(cli, nil)
	}
	if err != nil && len(result.Failures) == 0 {
		return signInFailed("Azure CLI sign-in failed; see its attached output", err)
	}
	return loginUserTenants(command, cli, args, deps, result.Failures)
}

func loginUserTenants(command *cobra.Command, cli azcli.CLI, args []string, deps Dependencies, failures []azcli.TenantFailure) error {
	if len(failures) != 0 {
		if err := printTenantFailures(command.ErrOrStderr(), failures); err != nil {
			return err
		}
		if !inputIsTerminal(command, deps) {
			return finishLogin(cli, failures)
		}
	}
	allTenants, _ := command.Flags().GetBool("all-tenants")
	var err error
	if allTenants {
		ids, err := cli.DiscoverTenants()
		if err != nil {
			if len(failures) != 0 {
				return signInFailed("tenant sign-in incomplete; tenant discovery failed", err)
			}
			return err
		}
		var tenants []azcli.TenantFailure
		for _, id := range ids {
			tenants = append(tenants, azcli.TenantFailure{ID: id})
		}
		failures, err = retryTenantLogin(command, cli, args, failures, tenants)
		if err != nil {
			return err
		}
	} else {
		failures, err = confirmAndRetryTenants(command, cli, args, failures)
		if err != nil {
			return err
		}
	}
	return finishLogin(cli, failures)
}

func confirmAndRetryTenants(command *cobra.Command, cli azcli.CLI, args []string, failures []azcli.TenantFailure) ([]azcli.TenantFailure, error) {
	retry := retryableTenants(failures)
	if len(retry) == 0 {
		return failures, nil
	}
	confirmed, err := confirmTenantLogin(command, len(retry))
	if err != nil || !confirmed {
		return failures, err
	}
	return retryTenantLogin(command, cli, args, failures, retry)
}

func retryableTenants(failures []azcli.TenantFailure) []azcli.TenantFailure {
	var retry []azcli.TenantFailure
	seen := make(map[string]bool)
	for _, failure := range failures {
		if !failure.Retryable() {
			seen[strings.ToLower(failure.ID)] = true
		}
	}
	for _, failure := range failures {
		id := strings.ToLower(failure.ID)
		if failure.Retryable() && !seen[id] {
			seen[id] = true
			retry = append(retry, failure)
		}
	}
	return retry
}

func confirmTenantLogin(command *cobra.Command, count int) (bool, error) {
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "Sign in to these %d tenants now? [Y/n] ", count); err != nil {
		return false, err
	}
	answer, err := readConfirmation(command.Context(), command.InOrStdin())
	if err != nil {
		return false, signInFailed("tenant sign-in incomplete; could not read confirmation", err)
	}
	if err := command.Context().Err(); err != nil {
		return false, err
	}
	answer = strings.TrimSpace(answer)
	return answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func retryTenantLogin(command *cobra.Command, cli azcli.CLI, args []string, failures, retry []azcli.TenantFailure) ([]azcli.TenantFailure, error) {
	var initialDefault profile.Subscription
	attempted := false
	for _, tenant := range retry {
		if err := command.Context().Err(); err != nil {
			return nil, err
		}
		if tenantRetryBlocked(tenant.ID, failures) {
			continue
		}
		if !attempted {
			// The initial login, not the pre-login profile, chooses the default.
			// A partial first login may not have created a readable profile yet.
			if cli.Reload() == nil {
				initialDefault, _ = cli.ActiveSubscription()
			}
			attempted = true
		}
		log.Info("Signing in to tenant %s.", tenant.ID)
		result, err := cli.LoginTenant(tenant.ID, args)
		if ExitCode(err) == ExitInterrupted {
			return nil, err
		}
		if command.Context().Err() != nil {
			return nil, command.Context().Err()
		}
		failures = recordTenantAttempt(failures, tenant, result, err)
		if len(result.Failures) != 0 {
			if err := printTenantFailures(command.ErrOrStderr(), result.Failures); err != nil {
				return nil, err
			}
		} else if err != nil {
			log.Error("Sign-in did not complete for tenant %s; see Azure CLI output.", tenant.ID)
		}
	}
	if err := command.Context().Err(); err != nil {
		return nil, err
	}
	if err := restoreLoginDefault(cli, initialDefault); err != nil {
		if len(failures) != 0 {
			return failures, signInFailed("tenant sign-in incomplete; could not restore the initial default subscription", err)
		}
		return failures, err
	}
	return failures, command.Context().Err()
}

func restoreLoginDefault(cli azcli.CLI, initial profile.Subscription) error {
	if initial.Id == "" {
		return nil
	}
	if err := cli.Reload(); err != nil {
		return err
	}
	current, err := cli.ActiveSubscription()
	if err == nil && strings.EqualFold(current.Id, initial.Id) {
		return nil
	}
	if err := cli.SetSubscription(initial); err != nil {
		return signInFailed("could not restore the initial default subscription", err)
	}
	return nil
}

func tenantRetryBlocked(id string, failures []azcli.TenantFailure) bool {
	for _, failure := range failures {
		if strings.EqualFold(failure.ID, id) && !failure.Retryable() {
			return true
		}
	}
	return false
}

func recordTenantAttempt(failures []azcli.TenantFailure, tenant azcli.TenantFailure, result azcli.LoginResult, err error) []azcli.TenantFailure {
	var remaining []azcli.TenantFailure
	for _, failure := range failures {
		if !strings.EqualFold(failure.ID, tenant.ID) {
			remaining = append(remaining, failure)
		}
	}
	targetFailed := false
	for _, failure := range result.Failures {
		if strings.EqualFold(failure.ID, tenant.ID) {
			targetFailed = true
		}
	}
	remaining = append(remaining, result.Failures...)
	if err != nil && !targetFailed {
		remaining = append(remaining, azcli.TenantFailure{
			ID: tenant.ID, Name: tenant.Name, Message: "Sign-in did not complete; see Azure CLI output.",
		})
	}
	return remaining
}

func printTenantFailures(output io.Writer, failures []azcli.TenantFailure) error {
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "TENANT ID\tNAME\tCODE\tDETAIL"); err != nil {
		return err
	}
	for _, failure := range failures {
		detail := failure.Message
		switch failure.Code {
		case "50076":
			detail = "MFA required"
		case "50079":
			detail = "MFA registration required"
		case "53003":
			detail = "Blocked by Conditional Access; retry will not help. Contact your tenant administrator."
		}
		if _, err := fmt.Fprintf(table, "%s\t%q\tAADSTS%s\t%s\n", failure.ID, failure.Name, failure.Code, detail); err != nil {
			return err
		}
	}
	return table.Flush()
}

func finishLogin(cli azcli.CLI, failures []azcli.TenantFailure) error {
	if err := cli.Reload(); err != nil {
		if len(failures) != 0 {
			return signInFailed("tenant sign-in incomplete; could not reload the Azure profile", err)
		}
		return err
	}
	if len(failures) != 0 {
		log.Info("Fetched %d tenants and %d subscriptions; sign-in incomplete.", cli.TenantCount(), len(cli.Subscriptions()))
		ids := make(map[string]bool)
		for _, failure := range failures {
			ids[strings.ToLower(failure.ID)] = true
		}
		return signInFailed(fmt.Sprintf("sign-in failed for %d tenants", len(ids)), nil)
	}
	log.Info("Successfully fetched %d tenants and %d subscriptions.", cli.TenantCount(), len(cli.Subscriptions()))
	return nil
}

func signInFailed(message string, cause error) error {
	return &ExitError{Code: CodeSignInFailed, Status: ExitSignInFailed, Message: message, Cause: cause}
}
