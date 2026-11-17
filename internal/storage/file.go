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
	for _, protected := range []string{azureDir, os.Getenv("AZURE_CONFIG_DIR")} {
		if protected == "" {
			continue
		}
		azure, err := canonical(protected)
		if err != nil {
			return err
		}
		compareTarget := target
		if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
			azure, compareTarget = strings.ToLower(azure), strings.ToLower(target)
		}
		if !strings.EqualFold(filepath.VolumeName(azure), filepath.VolumeName(compareTarget)) {
			continue
		}
		relative, err := filepath.Rel(azure, compareTarget)
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

// Remove deletes only a validated azctx private-file destination.
func Remove(path, azureDir string) error {
	if err := validate(path, azureDir); err != nil {
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
	if err := validate(path, azureDir); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create azctx directory: %w", err)
	}
	// Check again after directory creation, before changing permissions or writing.
	if err := validate(path, azureDir); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
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
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace azctx file: %w", err)
	}
	return nil
}
