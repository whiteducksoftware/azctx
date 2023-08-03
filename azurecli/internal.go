package azurecli

import (
	"fmt"
	"os"

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
	configFilePath := fmt.Sprintf("%s/%s", configDir, profilesJson)
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
