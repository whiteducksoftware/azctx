package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/whiteducksoftware/azctx/internal/storage"
)

func appendGitHubEnvironment(azureDir string, data []byte) error {
	path := os.Getenv("GITHUB_ENV")
	if path == "" {
		return errors.New("github format requires GITHUB_ENV to name an existing writable environment file")
	}
	if !filepath.IsAbs(path) {
		return errors.New("GITHUB_ENV must be an absolute file path")
	}
	if err := storage.Append(path, azureDir, data); err != nil {
		return fmt.Errorf("append GITHUB_ENV: %w", err)
	}
	return nil
}
