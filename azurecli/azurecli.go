package azurecli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/briandowns/spinner"
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

func (cli CLI) execLogin(extraArgs []string) error {
	// Create a buffer to store the stdErr output
	var stdErrBuffer bytes.Buffer

	// Execute the az login command
	args := []string{"login"}
	args = append(args, extraArgs...)
	err := utils.ExecuteCommandBare(AZ_COMMAND, io.Discard, &stdErrBuffer, args...)
	if err != nil {
		return err
	}

	// Print the stdErr output if it contains "WARNING:"
	stdErrString := stdErrBuffer.String()
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
		log.Error(strings.Repeat("-", 80))
		log.Error("Some tenants require Multi-factor authentication. Please run 'azctx login --force-mfa --' to login into each tenant separately using MFA.")
		log.Error(strings.Repeat("-", 80))
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
		return err
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

	// Fetch all available tenants
	tenants := cli.Tenants()
	if len(tenants) == 0 {
		return errors.New("no tenants found. Please run 'azctx login --' to normally login to Azure and fetch the tenants, after which you can try again logging in with MFA")
	}

	// Loop through the tenants and execute the az login command
	for _, tenant := range tenants {
		// Update the spinner suffix
		s.Suffix = " Logging in to tenant '" + tenant.Name + "'... Please check your browser for the login prompt."

		// Execute the az login command
		args := []string{"--tenant", tenant.Id}
		args = append(args, extraArgs...)
		err := cli.execLogin(args)
		if err != nil {
			log.Error("Failed to login to tenant '%s': %s", tenant.Name, err)
			continue
		}
	}

	return nil
}
