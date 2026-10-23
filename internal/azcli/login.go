package azcli

import (
	"context"
	"fmt"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

// capture preserves the legacy command error text for existing callers.
func (cli CLI) capture(args []string) (string, error) {
	result, err := cli.runner.Run(context.Background(), args, Options{})
	if err != nil {
		if result.Stderr != "" {
			return "", fmt.Errorf("Error executing command: %s: %w", result.Stderr, err)
		}
		return "", err
	}
	return result.Stdout, nil
}

func (cli CLI) execLogin(extraArgs []string) error {
	args := append([]string{"login"}, extraArgs...)
	if err := validateLoginIDs(extraArgs); err != nil {
		return err
	}
	terminal := cli.Terminal
	if terminal == nil {
		terminal = &Terminal{}
	}
	result, err := cli.runner.Run(context.Background(), args, Options{Terminal: terminal})
	if err != nil {
		var errs []string
		for _, line := range strings.Split(result.Stderr, "\n") {
			if strings.HasPrefix(line, "ERROR: ") {
				errs = append(errs, strings.TrimPrefix(line, "ERROR: "))
			}
		}
		if len(errs) != 0 {
			return fmt.Errorf("%s: %w", strings.Join(errs, "\n"), err)
		}
		return fmt.Errorf("az login: %w", err)
	}

	lower := strings.ToLower(result.Stderr)
	if strings.Contains(lower, "mfa") || strings.Contains(lower, "multi-factor authentication") {
		log.Error("%s", strings.Repeat("-", 80))
		log.Error("Some tenants require explicit MFA / Individual Authentication. Please run 'azctx login --force-mfa --' to login into each tenant separately.")
		log.Error("%s", strings.Repeat("-", 80))
	}
	return nil
}

func validateLoginIDs(args []string) error {
	for i, arg := range args {
		flag, value, hasValue := strings.Cut(arg, "=")
		if flag != "--subscription" && flag != "--tenant" && flag != "-t" {
			continue
		}
		if !hasValue && i+1 < len(args) {
			value = args[i+1]
		}
		if err := profile.ValidateGUID(value); err != nil {
			return fmt.Errorf("%s: %w", flag, err)
		}
	}
	return nil
}
