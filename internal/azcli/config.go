package azcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	configDirEnv = "AZURE_CONFIG_DIR"
	profileJSON  = "azureProfile.json"
	tenantsJSON  = "azctxTenants.json"
)

func resolveConfigDir(configDir string) (string, error) {
	if configDir == "" {
		configDir = os.Getenv(configDirEnv)
	}
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		configDir = filepath.Join(home, ".azure")
	}
	info, err := os.Stat(configDir) //nolint:gosec // AZURE_CONFIG_DIR intentionally selects the read-only profile directory.
	if err != nil {
		return "", fmt.Errorf("%s (%s) is not a valid directory. Please run `az login` and try again: %w", configDirEnv, configDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s (%s) is not a directory. Please run `az login` and try again", configDirEnv, configDir)
	}
	return configDir, nil
}

func (cli *CLI) readProfile() error {
	path := filepath.Join(cli.configDir, profileJSON)
	if _, err := fs.Stat(cli.fs, profileJSON); os.IsNotExist(err) {
		return fmt.Errorf("%s is not a valid file. Please run `az login` and try again: %w", path, err)
	}
	parsed, err := profile.Read(cli.fs, profileJSON)
	if err != nil {
		return err
	}
	cli.profile = parsed
	return nil
}

func (cli *CLI) readTenants() error {
	path := filepath.Join(cli.configDir, tenantsJSON)
	if _, err := fs.Stat(cli.fs, tenantsJSON); os.IsNotExist(err) {
		return nil
	}
	data, err := fs.ReadFile(cli.fs, tenantsJSON)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &cli.tenants); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", path, err)
	}
	return nil
}

// writeTenants preserves the existing legacy cache behavior until layer 08.
// No profile or credential files are written.
func (cli CLI) writeTenants() error {
	if !cli.diskConfig {
		return fs.ErrPermission
	}
	path := filepath.Join(cli.configDir, tenantsJSON)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("%s is not a valid file: %w", path, err)
	}
	defer file.Close()
	data, err := json.Marshal(cli.tenants)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}
