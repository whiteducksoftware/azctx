package azcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

const (
	configDirEnv = "AZURE_CONFIG_DIR"
	profileJSON  = "azureProfile.json"
	tenantsJSON  = "azctxTenants.json"
)

func (cli CLI) ensureConfigDir() (string, error) {
	configDir := cli.configDir
	if configDir == "" {
		configDir = os.Getenv(configDirEnv)
	}
	if configDir == "" {
		log.Warn("%s environment variable is not set. Using default config directory.", configDirEnv)
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %s", err.Error())
		}
		configDir = filepath.Join(home, ".azure")
	}
	if _, err := cli.fs.Stat(configDir); os.IsNotExist(err) {
		return "", fmt.Errorf("%s (%s) is not a valid directory. Please run `az configure` and try again.", configDirEnv, configDir)
	}
	return configDir, nil
}

func (cli *CLI) readProfile() error {
	configDir, err := cli.ensureConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(configDir, profileJSON)
	if _, err := cli.fs.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%s is not a valid file. Please run `az configure` and try again.", path)
	}
	files := afero.NewIOFS(afero.NewBasePathFs(cli.fs, configDir))
	parsed, err := profile.Read(files, profileJSON)
	if err != nil {
		return err
	}
	cli.profile = parsed
	return nil
}

func (cli *CLI) readTenants() error {
	configDir, err := cli.ensureConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(configDir, tenantsJSON)
	if _, err := cli.fs.Stat(path); os.IsNotExist(err) {
		return nil
	}
	data, err := afero.ReadFile(cli.fs, path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %s", path, err.Error())
	}
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &cli.tenants); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %s", path, err.Error())
	}
	return nil
}

// writeTenants preserves the existing legacy cache behavior until layer 08.
// No profile or credential files are written.
func (cli CLI) writeTenants() error {
	configDir, err := cli.ensureConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(configDir, tenantsJSON)
	file, err := cli.fs.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %s", path, err.Error())
	}
	defer file.Close()
	data, err := json.Marshal(cli.tenants)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %s", path, err.Error())
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %s", path, err.Error())
	}
	return nil
}
