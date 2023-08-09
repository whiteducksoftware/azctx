package azurecli

import (
	"bytes"
	"errors"
	"os"
	"strings"

	"github.com/whiteducksoftware/azctx/log"
	"github.com/whiteducksoftware/azctx/utils"

	"github.com/spf13/afero"
)

// New creates a new CLI instance
func New(fs afero.Fs) (CLI, error) {
	// Ensure that the azure cli is installed
	if !utils.IsCommandInstalled(AZ_COMMAND) {
		return CLI{}, errors.New("azure cli is not installed. please install it and try again. See here: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli")
	}

	// Create a new CLI instance
	cli := CLI{
		fs:      fs,
		profile: Profile{},
		tenants: make([]Tenant, 0),
	}

	// Refresh the CLI instance
	err := cli.Refresh()
	if err != nil {
		return CLI{}, err
	}

	// Map the tenant ids to the tenant names
	cli.MapTenantIdsToNames()

	return cli, nil
}

// Refresh refreshes the CLI instance by fetching the latest data from the azure cli
func (cli *CLI) Refresh() error {
	// Read the azureProfile.json file
	err := cli.readProfile()
	if err != nil {
		return err
	}

	// Read the azctxTenants.json file
	err = cli.readTenants()
	if err != nil {
		return err
	}

	return nil
}

// Login executes the az login command
func (cli CLI) Login(extraArgs []string) error {
	// Create a buffer to store the stdErr output
	var stdErrBuffer bytes.Buffer

	// Execute the az login command
	args := []string{"login"}
	args = append(args, extraArgs...)
	err := utils.ExecuteCommandBare(AZ_COMMAND, os.Stdout, &stdErrBuffer, args...)
	if err != nil {
		log.Error("Failed to execute 'az login': %s", err)
		return err
	}

	// Convert the stdErr buffer to a string
	stdErrString := stdErrBuffer.String()

	// Only print lines starting with "WARNING:" to the user
	lines := strings.Split(stdErrString, "\n")
	for _, line := range lines {
		// Check if the line starts with "WARNING:" but ignore the "A web browser has been opened at" line
		if strings.HasPrefix(line, "WARNING:") && !strings.Contains(line, "A web browser has been opened at") {
			// Remove the "WARNING: " prefix and print the line
			log.Warn(strings.TrimPrefix(line, "WARNING: "))
		}
	}

	// Check if the output contains "mfa" or "multi-factor authentication"
	stdErrLower := strings.ToLower(stdErrString)
	if strings.Contains(stdErrLower, "mfa") || strings.Contains(stdErrLower, "multi-factor authentication") {
		log.Warn(strings.Repeat("-", 80))
		log.Warn("Multi-factor authentication is required. Please run 'azctx login -- --tenant TENANT_ID' to login to a specific tenant using MFA. See above output for the tenant ids.")
		log.Warn(strings.Repeat("-", 80))
	}

	return nil
}
