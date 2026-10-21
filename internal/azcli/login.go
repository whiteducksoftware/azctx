package azcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	loginSubscriptionFlag = "--subscription"
	loginTenantFlag       = "--tenant"
	loginShortTenantFlag  = "-t"
)

// UsesWorkloadIdentity identifies explicit non-user authentication passthrough.
func UsesWorkloadIdentity(args []string) bool {
	identity, principal := false, false
	for i := 0; i < len(args); i++ {
		flag, value, inline := strings.Cut(args[i], "=")
		enabled, _ := strconv.ParseBool(value)
		switch flag {
		case "--identity", "-i":
			identity = !inline || enabled
		case "--service-principal":
			principal = !inline || enabled
		case "--password", "-p", "--username", "-u", "--federated-token", "--certificate",
			"--client-id", "--object-id", "--resource-id", loginTenantFlag, loginShortTenantFlag, loginSubscriptionFlag, "--scope":
			if !inline {
				i++
			}
		}
	}
	return identity || principal
}

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

func (cli CLI) execLogin(extraArgs []string) (LoginResult, error) {
	if err := cli.context().Err(); err != nil {
		return LoginResult{}, err
	}
	args := append([]string{"login"}, extraArgs...)
	if err := validateLoginIDs(extraArgs); err != nil {
		return LoginResult{}, err
	}
	terminal := cli.Terminal
	if terminal == nil {
		terminal = &Terminal{}
	}
	result, err := cli.runner.Run(cli.context(), args, Options{
		Terminal: terminal, Env: []string{"AZURE_CORE_LOGIN_EXPERIENCE_V2=off"},
	})
	login := LoginResult{Attempted: true, Failures: parseTenantFailures(result.Stderr)}
	if cli.context().Err() != nil {
		return login, errors.Join(cli.context().Err(), err)
	}
	if result.ExitCode == 130 {
		return login, errors.Join(context.Canceled, err)
	}
	if err != nil {
		var errs []string
		for _, line := range strings.Split(result.Stderr, "\n") {
			if strings.HasPrefix(line, "ERROR: ") {
				errs = append(errs, strings.TrimPrefix(line, "ERROR: "))
			}
		}
		if len(errs) != 0 {
			return login, fmt.Errorf("%s: %w", strings.Join(errs, "\n"), err)
		}
		return login, fmt.Errorf("az login: %w", err)
	}
	return login, nil
}

// LoginTenant adds one tenant to a login already version-checked by Login.
// Existing tenant selectors are replaced, never duplicated or sent as names.
func (cli CLI) LoginTenant(id string, extraArgs []string) (LoginResult, error) {
	if err := profile.ValidateGUID(id); err != nil {
		return LoginResult{}, fmt.Errorf("invalid tenant: %w", err)
	}
	args := []string{loginTenantFlag, id}
	for i := 0; i < len(extraArgs); i++ {
		flag, _, inline := loginIDOption(extraArgs[i])
		if flag == loginTenantFlag || flag == loginShortTenantFlag {
			if !inline {
				i++
			}
			continue
		}
		args = append(args, extraArgs[i])
	}
	return cli.execLogin(args)
}

func (cli CLI) context() context.Context {
	if cli.Context != nil {
		return cli.Context
	}
	return context.Background()
}

func validateLoginIDs(args []string) error {
	for i, arg := range args {
		flag, value, hasValue := loginIDOption(arg)
		if flag != loginSubscriptionFlag && flag != loginTenantFlag && flag != loginShortTenantFlag {
			continue
		}
		if !hasValue && i+1 < len(args) {
			value = args[i+1]
		}
		if err := profile.ValidateGUID(value); err != nil {
			return fmt.Errorf("%s requires a valid GUID", flag)
		}
	}
	return nil
}

func loginIDOption(arg string) (flag, value string, inline bool) {
	flag, value, inline = strings.Cut(arg, "=")
	if strings.HasPrefix(flag, loginShortTenantFlag) && flag != loginShortTenantFlag && !strings.HasPrefix(flag, "--") {
		return loginShortTenantFlag, arg[2:], true
	}
	return flag, value, inline
}
