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
		if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
			return "", fmt.Errorf("check Azure CLI version: %s: %w", stderr, err)
		}
		return "", fmt.Errorf("check Azure CLI version: %w", err)
	}
	var data struct {
		Version string `json:"azure-cli"` //nolint:tagliatelle // Azure CLI's version output uses this hyphenated key.
	}
	if err := json.Unmarshal([]byte(result.Stdout), &data); err != nil {
		return "", fmt.Errorf("invalid az version JSON: %w", err)
	}
	numbers, err := parseVersion(data.Version)
	if err != nil {
		return "", err
	}
	if numbers[0] < 2 || numbers[0] == 2 && numbers[1] < 61 {
		return data.Version, fmt.Errorf("Azure CLI %s or newer is required; found %s", MinimumVersion, data.Version)
	}
	return data.Version, nil
}

func parseVersion(version string) ([3]int, error) {
	parts := strings.Split(version, ".")
	var numbers [3]int
	if len(parts) != len(numbers) {
		return numbers, fmt.Errorf("invalid Azure CLI version %q; require %s or newer", version, MinimumVersion)
	}
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return numbers, fmt.Errorf("invalid Azure CLI version %q; require %s or newer", version, MinimumVersion)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return numbers, fmt.Errorf("invalid Azure CLI version %q: %w", version, err)
		}
		numbers[i] = value
	}
	return numbers, nil
}
