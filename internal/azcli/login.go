package azcli

import (
	"context"
	"errors"
	"strings"

	"github.com/whiteducksoftware/azctx/log"
)

// capture preserves the legacy command error text for existing callers.
func (cli CLI) capture(args []string) (string, error) {
	result, err := cli.runner.Run(context.Background(), args, Options{})
	if err != nil {
		if result.Stderr != "" {
			return "", errors.New("Error executing command: " + result.Stderr)
		}
		return "", err
	}
	return result.Stdout, nil
}

func (cli CLI) execLogin(extraArgs []string) error {
	args := append([]string{"login"}, extraArgs...)
	// Login stays captured in this layer; live device-code output belongs to
	// the authentication layer. Attached mode is available at the runner seam.
	result, err := cli.runner.Run(context.Background(), args, Options{})
	if err != nil {
		var errs []string
		for _, line := range strings.Split(result.Stderr, "\n") {
			if strings.HasPrefix(line, "ERROR: ") {
				errs = append(errs, strings.TrimPrefix(line, "ERROR: "))
			}
		}
		return errors.New(strings.Join(errs, "\n"))
	}
	for _, line := range strings.Split(result.Stderr, "\n") {
		if strings.HasPrefix(line, "WARNING:") && !strings.Contains(line, "A web browser has been opened at") {
			log.Warn("%s", strings.TrimPrefix(line, "WARNING: "))
		}
	}
	lower := strings.ToLower(result.Stderr)
	if strings.Contains(lower, "mfa") || strings.Contains(lower, "multi-factor authentication") {
		log.Error("%s", strings.Repeat("-", 80))
		log.Error("Some tenants require explicit MFA / Individual Authentication. Please run 'azctx login --force-mfa --' to login into each tenant separately.")
		log.Error("%s", strings.Repeat("-", 80))
	}
	return nil
}
