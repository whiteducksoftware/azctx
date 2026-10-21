package azcli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const MinimumVersion = "2.61.0"

// CheckVersion is shared by login and future diagnostics. It is deliberately not
// part of loading a profile or switching subscriptions.
func CheckVersion(ctx context.Context, runner Runner) (string, error) {
	result, err := runner.Run(ctx, []string{"version", "-o", "json"}, Options{})
	if err != nil {
		return "", fmt.Errorf("check Azure CLI version: %s: %w", strings.TrimSpace(result.Stderr), err)
	}
	var data struct {
		Version string `json:"azure-cli"` //nolint:tagliatelle // Azure CLI's version output uses this hyphenated key.
	}
	if err := json.Unmarshal([]byte(result.Stdout), &data); err != nil {
		return "", fmt.Errorf("invalid az version JSON: %w", err)
	}
	parts := strings.Split(data.Version, ".")
	var numbers [3]int
	if len(parts) != len(numbers) {
		return "", fmt.Errorf("invalid Azure CLI version %q; require %s or newer", data.Version, MinimumVersion)
	}
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return "", fmt.Errorf("invalid Azure CLI version %q; require %s or newer", data.Version, MinimumVersion)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return "", fmt.Errorf("invalid Azure CLI version %q: %w", data.Version, err)
		}
		numbers[i] = value
	}
	if numbers[0] < 2 || numbers[0] == 2 && numbers[1] < 61 {
		return data.Version, fmt.Errorf("Azure CLI %s or newer is required; found %s", MinimumVersion, data.Version)
	}
	return data.Version, nil
}
