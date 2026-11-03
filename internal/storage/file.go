// Package storage keeps azctx's own private files outside Azure configuration.
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const openOp = "open"

// ConfigPath resolves azctx-owned configuration and state, never Azure files.
// AZCTX_CONFIG_DIR names the directory itself, not a parent of "azctx".
func ConfigPath(azureDir, name string) (string, error) {
	dir := os.Getenv("AZCTX_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "azctx")
	}
	path := filepath.Join(dir, name)
	if err := validate(path, azureDir); err != nil {
		return "", err
	}
	return path, nil
}

// Path validates a state/cache destination, including existing symlink parents.
// It never creates the directory.
func Path(base, azureDir, name string) (string, error) {
	path := filepath.Join(base, "azctx", name)
	if err := validate(path, azureDir); err != nil {
		return "", err
	}
	return path, nil
}

func validate(path, azureDir string) error {
	if azureDir == "" {
		azureDir = os.Getenv("AZURE_CONFIG_DIR")
	}
	if azureDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		azureDir = filepath.Join(home, ".azure")
	}
	target, err := canonical(path)
	if err != nil {
		return err
	}
	parent, err := canonical(filepath.Dir(path))
	if err != nil {
		return err
	}
	// Replacing or removing a symlink changes its entry, not its target.
	entry := filepath.Join(parent, filepath.Base(path))
	for _, protected := range []string{azureDir, os.Getenv("AZURE_CONFIG_DIR")} {
		if protected == "" {
			continue
		}
		azure, err := canonical(protected)
		if err != nil {
			return err
		}
		if err := validateOutsideAzure(path, azure, entry, target); err != nil {
			return err
		}
	}
	return nil
}

func validateOutsideAzure(path, azure string, targets ...string) error {
	for _, target := range targets {
		compareTarget := target
		compareAzure := azure
		if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
			compareAzure, compareTarget = strings.ToLower(azure), strings.ToLower(target)
		}
		if !strings.EqualFold(filepath.VolumeName(compareAzure), filepath.VolumeName(compareTarget)) {
			continue
		}
		relative, err := filepath.Rel(compareAzure, compareTarget)
		if err != nil {
			return err
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("refusing azctx storage inside Azure configuration: %s", path)
		}
	}
	return nil
}

func canonical(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent := path
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			suffix, err := filepath.Rel(parent, path)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, suffix), nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(parent) == parent {
			return "", err
		}
		parent = filepath.Dir(parent)
	}
}

func privateFileInfo(path, azureDir string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if info != nil && !info.Mode().IsRegular() {
		return nil, errors.New("azctx storage destination must be a regular file, not a symlink")
	}
	if err := validate(path, azureDir); err != nil {
		return nil, err
	}
	return info, nil
}

// Remove deletes only a validated azctx private-file destination.
func Remove(path, azureDir string) error {
	if _, err := privateFileInfo(path, azureDir); err != nil {
		return err
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Write atomically replaces a private file using a same-directory staging file.
func Write(path, azureDir string, data []byte) error {
	if _, err := privateFileInfo(path, azureDir); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create azctx directory: %w", err)
	}
	// Check again after directory creation, before changing permissions or writing.
	if _, err := privateFileInfo(path, azureDir); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return replace(path, azureDir, data)
}

func replace(path, azureDir string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".azctx-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := privateFileInfo(path, azureDir); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace azctx file: %w", err)
	}
	return nil
}
