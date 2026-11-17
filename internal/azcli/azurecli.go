// Package azcli delegates Azure operations to an injected Runner and reads the
// Azure CLI configuration without handling credentials.
package azcli

import (
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/afero"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

// New loads the profile and legacy tenant cache using an already resolved runner.
// An empty configDir selects AZURE_CONFIG_DIR or the user's .azure directory.
func New(fs afero.Fs, runner Runner, configDir string) (CLI, error) {
	cli := CLI{
		fs:                       fs,
		runner:                   runner,
		configDir:                configDir,
		profile:                  profile.Profile{},
		tenants:                  make([]Tenant, 0),
		FilterTenantLevelAccount: true,
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
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithHiddenCursor(false))
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
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithHiddenCursor(false))
	s.Color("green", "italic", "bold")
	s.Start()
	defer s.Stop()

	// Loop through the tenants and execute the az login command
	tenants := cli.Tenants()
	for _, tenant := range tenants {
		// Update the spinner suffix
		s.Suffix = " Logging into tenant " + color.HiGreenString(tenant.Name) + "... Please check your browser for the login prompt."

		// Execute the az login command
		args := []string{"--tenant", tenant.Id}
		args = append(args, extraArgs...)
		err := cli.execLogin(args)
		if err != nil {
			fmt.Println() // Force a new line after the spinner
			log.Error("Failed to login to tenant %s. Azure CLI returned the following error: %s", tenant.Name, color.RedString(err.Error()))
			continue
		}
	}

	return nil
}

// SetSubscription asks az to set the default subscription.
func (cli CLI) SetSubscription(subscription profile.Subscription) error {
	// Execute az account set command
	args := []string{"account", "set", "--subscription", subscription.Id}
	_, err := cli.capture(args)
	if err != nil {
		return err
	}

	return nil
}
