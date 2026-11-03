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

// OpenConfig resolves a read-only Azure filesystem without requiring a profile
// or existing directory. It does not create any files or directories.
func OpenConfig(files fs.FS, configDir string) (fs.FS, string, error) {
	if files != nil {
		return files, configDir, nil
	}
	directory, err := resolveConfigDir(configDir, true)
	if err != nil {
		return nil, directory, err
	}
	return os.DirFS(directory), directory, nil
}

func resolveConfigDir(configDir string, allowMissing bool) (string, error) {
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
	if allowMissing && os.IsNotExist(err) {
		return configDir, nil
	}
	if err != nil {
		return configDir, fmt.Errorf("%s (%s) is not a valid directory. Please run `az login` and try again: %w", configDirEnv, configDir, err)
	}
	if !info.IsDir() {
		return configDir, fmt.Errorf("%s (%s) is not a directory. Please run `az login` and try again", configDirEnv, configDir)
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
