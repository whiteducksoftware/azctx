package azcli

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const MacBrokerMinimumVersion = "2.91.0"

const platformWindows = "windows"

// BrokerInfo describes effective settings, not enrollment or token availability.
type BrokerInfo struct {
	Platform       string `json:"platform"`
	Supported      bool   `json:"supported"`
	Available      bool   `json:"available"`
	Setting        string `json:"setting"`
	Enabled        bool   `json:"enabled"`
	Preview        bool   `json:"preview"`
	MinimumVersion string `json:"minimumVersion"`
}

// BrokerStatus uses Azure CLI's environment-aware settings and platform defaults.
// An empty/invalid version means that broker availability is not established.
func BrokerStatus(settings Settings, platform, azureVersion string) (BrokerInfo, error) {
	info := BrokerInfo{Platform: platform, MinimumVersion: MinimumVersion}
	switch platform {
	case platformWindows:
		info.Supported, info.Setting = true, "core.enable_broker_on_windows"
	case "darwin":
		info.Supported, info.Preview = true, true
		info.Setting, info.MinimumVersion = "core.enable_broker_on_mac", MacBrokerMinimumVersion
	default:
		return info, nil
	}
	_, key, _ := strings.Cut(info.Setting, ".")
	enabled, err := settings.Bool("core", key, platform == platformWindows)
	info.Enabled = enabled
	numbers, versionErr := parseVersion(azureVersion)
	minimum, _ := parseVersion(info.MinimumVersion)
	info.Available = versionErr == nil && slices.Compare(numbers[:], minimum[:]) >= 0
	return info, err
}

// LoginPrompt describes the effective browser/broker/device-code path.
func LoginPrompt(info BrokerInfo, args []string) string {
	if UsesDeviceCode(args) {
		return "Follow the Azure CLI device code instructions."
	}
	if UsesWorkloadIdentity(args) {
		return "Follow the Azure CLI sign-in instructions."
	}
	if info.Available && info.Enabled {
		if info.Platform == platformWindows {
			return "Use the Windows Web Account Manager (WAM) account picker."
		}
		return "Use the macOS broker account picker (preview; requires an enrolled device with the Enterprise SSO plug-in)."
	}
	return "Please check your browser for the login prompt."
}

// SetBroker runs the documented clear/config/login sequence. Callers must first
// check capability, reject conflicting environment overrides and obtain explicit
// confirmation. Only Azure CLI writes its own files; partial failures are not
// rolled back because accounts may already have been cleared.
func SetBroker(ctx context.Context, runner Runner, platform string, enabled bool, terminal *Terminal) error {
	info, err := BrokerStatus(Settings{}, platform, MacBrokerMinimumVersion)
	if err != nil || !info.Supported {
		return fmt.Errorf("authentication broker is not supported on %s", platform)
	}
	cli := CLI{runner: runner, Terminal: terminal, Context: ctx}
	for _, args := range [][]string{
		{"account", "clear"},
		{"config", "set", fmt.Sprintf("%s=%t", info.Setting, enabled)},
	} {
		if err := cli.capture(args); err != nil {
			return fmt.Errorf("az %s failed; broker change stopped (account/config changes may already have applied): %w", strings.Join(args[:2], " "), err)
		}
	}
	if _, err := cli.execLogin(nil); err != nil {
		return fmt.Errorf("broker setting persisted but sign-in did not complete; retry azctx login: %w", err)
	}
	return nil
}
