package azcli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

const (
	configDirEnv = "AZURE_CONFIG_DIR"
	profileJSON  = "azureProfile.json"
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
