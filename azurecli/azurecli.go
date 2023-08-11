package azurecli

import (
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
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
	err := cli.Reload()
	if err != nil {
		return CLI{}, err
	}

	// Map the tenant ids to the tenant names
	cli.MapTenantIdsToNames()

	return cli, nil
}

// Reload refreshes the CLI instance by fetching the latest data from the azure cli config files
func (cli *CLI) Reload() error {
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

// InteractiveLogin executes the az login command
func (cli CLI) InteractiveLogin(extraArgs []string) error {
	// Create a spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Color("green", "italic", "bold")
	s.Suffix = " Logging in... Please check your browser for the login prompt."
	s.Start()
	defer s.Stop()

	// Execute the az login command
	err := cli.execLogin(extraArgs)
	if err != nil {
		return errors.New("failed to login: " + err.Error())
	}

	return nil
}

// IterativeTenantLogin explicitly executes the az login command for each tenant
func (cli CLI) IterativeTenantLogin(extraArgs []string) error {
	// Create a spinner
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Color("green", "italic", "bold")
	s.Start()
	defer s.Stop()

	// Loop through the tenants and execute the az login command
	tenantStringColor := color.New(color.FgHiGreen)
	tenants := cli.Tenants()
	for _, tenant := range tenants {
		// Update the spinner suffix
		s.Suffix = " Logging into tenant " + tenantStringColor.Sprint(tenant.Name) + "... Please check your browser for the login prompt."

		// Execute the az login command
		args := []string{"--tenant", tenant.Id}
		args = append(args, extraArgs...)
		err := cli.execLogin(args)
		if err != nil {
			fmt.Println() // Force a new line after the spinner
			log.Error("Failed to login to tenant '%s': %s", tenant.Name, err)
			continue
		}
	}

	return nil
}
