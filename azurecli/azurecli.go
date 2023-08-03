package azurecli

import (
	"errors"
	"os"

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
	args := []string{"login"}
	args = append(args, extraArgs...)
	err := utils.ExecuteCommandBare(AZ_COMMAND, os.Stdout, os.Stderr, args...)
	if err != nil {
		return err
	}

	return nil
}
