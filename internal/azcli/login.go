package azcli

import (
	"context"
	"fmt"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

// UsesDeviceCode reports whether the effective passthrough flag requests a code.
func UsesDeviceCode(args []string) bool {
	enabled := false
	for _, arg := range args {
		switch arg {
		case "--use-device-code", "--use-device-code=true":
			enabled = true
		case "--use-device-code=false":
			enabled = false
		}
	}
	return enabled
}

// capture preserves the legacy command error text for existing callers.
func (cli CLI) capture(args []string) error {
	result, err := cli.runner.Run(cli.context(), args, Options{})
	if err != nil {
		if result.Stderr != "" {
			return fmt.Errorf("Error executing command: %s: %w", result.Stderr, err)
		}
		return err
	}
	return nil
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
	result, err := cli.runner.Run(cli.context(), args, Options{
		Terminal: terminal, Env: []string{"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"},
	})
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

func (cli CLI) context() context.Context {
	if cli.Context != nil {
		return cli.Context
	}
	return context.Background()
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
