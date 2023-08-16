package azurecli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/whiteducksoftware/azctx/log"
	"github.com/whiteducksoftware/azctx/utils"
)

const (
	AZ_COMMAND     = "az"
	CONFIG_DIR_ENV = "AZURE_CONFIG_DIR"
	PROFILES_JSON  = "azureProfile.json"
	TENANTS_JSON   = "azctxTenants.json"
)

// ensureConfigDir ensures that the config dir exists
func ensureConfigDir() (string, error) {
	// Verify that the AZURE_CONFIG_DIR environment variable is set
	configDir := os.Getenv(CONFIG_DIR_ENV)
	if configDir == "" {
		log.Warn("%s environment variable is not set. Using default config directory.", CONFIG_DIR_ENV)

		// Get the user's home directory
		userhomeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %s", err.Error())
		}
		configDir = fmt.Sprintf("%s/.azure", userhomeDir)
	}

	// Verify that the config dir exists
	if !utils.FileExists(configDir) {
		return "", fmt.Errorf("%s (%s) is not a valid directory. Please run `az configure` and try again.", CONFIG_DIR_ENV, configDir)
	}

	return configDir, nil
}

// readProfiles reads the profiles from the azureProfile.json file
func (cli *CLI) readProfile() error {
	// Ensure that the config dir exists
	configDir, err := ensureConfigDir()
	if err != nil {
		return err
	}

	// Verify that the azureProfile.json file exists
	configFilePath := fmt.Sprintf("%s/%s", configDir, PROFILES_JSON)
	if !utils.FileExists(configFilePath) {
		return fmt.Errorf("%s is not a valid file. Please run `az configure` and try again.", configFilePath)
	}

	// Open the azureProfile.json file
	configFile, err := cli.fs.OpenFile(configFilePath, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %s", configFilePath, err.Error())
	}

	// Unmarshal the config file
	err = utils.ReadJson(configFile, &cli.profile)
	if err != nil {
		configFile.Close()
		return err
	}

	configFile.Close()
	return nil
}

/*
// writeProfile writes the profile to the azureProfile.json file
func (cli CLI) writeProfile() error {
	// Ensure that the config dir exists
	configDir, err := ensureConfigDir()
	if err != nil {
		return err
	}

	// Open the azureProfile.json file
	configFilePath := fmt.Sprintf("%s/%s", configDir, PROFILES_JSON)
	configFile, err := cli.fs.OpenFile(configFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %s", configFilePath, err.Error())
	}

	// Marshal the config file
	err = utils.WriteJson(configFile, cli.profile)
	if err != nil {
		configFile.Close()
		return err
	}

	configFile.Close()
	return nil
}
*/

// readTenants reads the tenants from the azctxTenants.json file
func (cli *CLI) readTenants() error {
	// Ensure that the config dir exists
	configDir, err := ensureConfigDir()
	if err != nil {
		return err
	}

	// Verify that the azctxTenants.json file exists
	configFilePath := fmt.Sprintf("%s/%s", configDir, TENANTS_JSON)
	if !utils.FileExists(configFilePath) {
		// ignore if the file does not exist
		return nil
	}

	// Open the azctxTenants.json file
	configFile, err := cli.fs.OpenFile(configFilePath, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %s", configFilePath, err.Error())
	}

	// Unmarshal the config file
	err = utils.ReadJson(configFile, &cli.tenants)
	if err != nil {
		configFile.Close()
		return err
	}

	configFile.Close()
	return nil
}

// writeTenants writes the tenants to the azctxTenants.json file
func (cli CLI) writeTenants() error {
	// Ensure that the config dir exists
	configDir, err := ensureConfigDir()
	if err != nil {
		return err
	}

	// Open the azctxTenants.json file
	configFilePath := fmt.Sprintf("%s/%s", configDir, TENANTS_JSON)
	configFile, err := cli.fs.OpenFile(configFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %s", configFilePath, err.Error())
	}

	// Marshal the config file
	err = utils.WriteJson(configFile, cli.tenants)
	if err != nil {
		configFile.Close()
		return err
	}

	configFile.Close()
	return nil
}

// execLogin executes the az login command
func (cli CLI) execLogin(extraArgs []string) error {
	// Create a buffer to store the stdErr output
	var stdErrBuffer bytes.Buffer

	// Execute the az login command
	args := []string{"login"}
	args = append(args, extraArgs...)
	err := utils.ExecuteCommandBare(AZ_COMMAND, io.Discard, &stdErrBuffer, args...)
	if err != nil {
		// Only return lines which start with "ERROR: "
		errs := make([]string, 0)
		stdErrString := stdErrBuffer.String()
		lines := strings.Split(stdErrString, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "ERROR: ") {
				errs = append(errs, strings.TrimPrefix(line, "ERROR: "))
			}
		}

		return errors.New(strings.Join(errs, "\n"))
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
		log.Error("Some tenants require explicit MFA / Individual Authentication. Please run 'azctx login --force-mfa --' to login into each tenant separately.")
		log.Error(strings.Repeat("-", 80))
	}

	return nil
}
