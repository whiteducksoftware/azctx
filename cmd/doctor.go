package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/redact"
	"github.com/whiteducksoftware/azctx/internal/update"
	"github.com/whiteducksoftware/azctx/internal/version"
)

// DoctorReport is a separate diagnostic projection, not a subscription DTO.
// Only allowlisted settings and account-type counts are included.
type DoctorReport struct {
	SchemaVersion    int                  `json:"schemaVersion"`
	AzctxVersion     string               `json:"azctxVersion"`
	Update           update.Status        `json:"update"`
	AzureCLI         DoctorAzureCLI       `json:"azureCli"`
	ConfigDirectory  string               `json:"configDirectory"`
	Profile          DoctorProfile        `json:"profile"`
	LegacyTenantFile DoctorLegacyFile     `json:"legacyTenantFile"`
	Settings         DoctorSettings       `json:"settings"`
	ActiveCloud      string               `json:"activeCloud"`
	AccountTypes     []DoctorAccountCount `json:"accountTypes"`
	Warnings         []string             `json:"warnings"`
}

type DoctorAzureCLI struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type DoctorProfile struct {
	Status        string `json:"status"`
	Subscriptions int    `json:"subscriptions"`
}

type DoctorLegacyFile struct {
	Status        string `json:"status"`
	DeleteCommand string `json:"deleteCommand"`
}

type DoctorSettings struct {
	Status            string            `json:"status"`
	Broker            *azcli.BrokerInfo `json:"broker"`
	LoginExperienceV2 *bool             `json:"loginExperienceV2"`
}

type DoctorAccountCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

const (
	diagnosticOK          = "ok"
	diagnosticUnavailable = "unavailable"
	diagnosticInvalid     = "invalid"
	legacyTenantFilename  = "azctxTenants.json"
)

func newDoctorCommand(resolve runnerFactory, deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Show safe, offline Azure CLI diagnostics", Args: cobra.NoArgs,
		RunE: commandHandler(func(command *cobra.Command, _ []string) error {
			if err := validateOutput(outputFormat(command), "doctor", formatText, formatJSON); err != nil {
				return err
			}
			report, err := doctorReport(command, resolve, deps)
			if err != nil {
				return err
			}
			if outputFormat(command) == formatJSON {
				return json.NewEncoder(command.OutOrStdout()).Encode(report)
			}
			return writeDoctorText(command.OutOrStdout(), report)
		}),
	}
}

func doctorReport(command *cobra.Command, resolve runnerFactory, deps Dependencies) (DoctorReport, error) {
	report := DoctorReport{
		SchemaVersion: 1, AzctxVersion: version.Get().Version,
		AccountTypes: []DoctorAccountCount{}, Warnings: []string{},
	}
	var err error
	report.AzureCLI, err = diagnoseAzure(command, resolve, deps.ConfigDir)
	if err != nil {
		return report, err
	}
	switch report.AzureCLI.Status {
	case "unsupported":
		report.Warnings = append(report.Warnings, "Azure CLI 2.61.0 or newer is required for sign-in.")
	case diagnosticUnavailable:
		report.Warnings = append(report.Warnings, "Azure CLI is missing or its version could not be determined; install or repair Azure CLI.")
	}
	if deps.Platform == "darwin" {
		capability, _ := azcli.BrokerStatus(azcli.Settings{}, deps.Platform, report.AzureCLI.Version)
		if !capability.Available {
			report.Warnings = append(report.Warnings, "macOS broker preview requires Azure CLI 2.91.0 or newer and an enrolled device with the Enterprise SSO plug-in.")
		}
	}
	files, directory, configErr := azcli.OpenConfig(deps.FS, deps.ConfigDir)
	report.ConfigDirectory = directory
	report.Update = diagnoseUpdate(directory, report.AzctxVersion, &report.Warnings)
	if configErr != nil {
		report.Profile.Status, report.Settings.Status, report.LegacyTenantFile.Status = diagnosticUnavailable, diagnosticUnavailable, diagnosticUnavailable
		report.Warnings = append(report.Warnings, "Azure configuration directory is unavailable.")
	} else {
		report.Profile, report.AccountTypes = diagnoseProfile(files)
		if report.Profile.Status != diagnosticOK {
			report.Warnings = append(report.Warnings, "Azure profile is missing, unreadable or invalid; run azctx login to create or refresh it.")
		}
		report.LegacyTenantFile = diagnoseLegacyFile(files, directory, deps.Platform)
		if report.LegacyTenantFile.Status == "present" {
			report.Warnings = append(report.Warnings, "A leftover azctxTenants.json can be deleted manually; azctx does not read it.")
		}
		report.Settings, report.ActiveCloud = diagnoseSettings(files, deps.Platform, report.AzureCLI.Version)
		if report.Settings.Status != diagnosticOK {
			report.Warnings = append(report.Warnings, "Azure settings are unreadable, malformed or contain an invalid authentication boolean; inspect config and AZURE_CORE overrides.")
		}
	}
	if err := command.Context().Err(); err != nil {
		return report, err
	}
	report.ConfigDirectory = redact.Text(report.ConfigDirectory)
	report.ActiveCloud = redact.Text(report.ActiveCloud)
	report.LegacyTenantFile.DeleteCommand = redact.Text(report.LegacyTenantFile.DeleteCommand)
	return report, nil
}

func diagnoseAzure(command *cobra.Command, resolve runnerFactory, configDir string) (DoctorAzureCLI, error) {
	info := DoctorAzureCLI{Status: diagnosticUnavailable}
	runner, err := resolve()
	if err != nil {
		return info, nil //nolint:nilerr // Doctor succeeds offline without az; never expose raw lookup diagnostics.
	}
	if resolved, ok := runner.(interface{ Path() string }); ok {
		info.Path = redact.Text(resolved.Path())
	}
	azureVersion, err := azcli.CheckVersion(command.Context(), azcli.WithConfigDir(runner, configDir))
	if ExitCode(err) == ExitInterrupted {
		return info, &ExitError{Code: CodeInterrupted, Message: "interrupted", Status: ExitInterrupted, Cause: err}
	}
	if err == nil {
		info.Status = diagnosticOK
	} else if azureVersion != "" {
		info.Status = "unsupported"
	}
	// CheckVersion returns a nonempty version only after validating its syntax.
	info.Version = azureVersion
	return info, nil
}

func diagnoseUpdate(directory, current string, warnings *[]string) update.Status {
	status, err := (update.Checker{AzureDir: directory, Current: current}).CachedStatus()
	if err != nil {
		status.State = diagnosticUnavailable
		*warnings = append(*warnings, "Update cache is unavailable; doctor does not perform network update checks.")
	}
	return status
}

func diagnoseProfile(files fs.FS) (DoctorProfile, []DoctorAccountCount) {
	parsed, err := profile.Read(files, "azureProfile.json")
	if err != nil {
		status := diagnosticInvalid
		if errors.Is(err, fs.ErrNotExist) {
			status = "missing"
		}
		return DoctorProfile{Status: status}, []DoctorAccountCount{}
	}
	counts := make(map[string]int)
	for _, subscription := range parsed.Subscriptions {
		account := subscription.AccountType()
		switch account {
		case "user", "service principal", "system-assigned identity", "user-assigned identity":
		default:
			account = "unknown"
		}
		counts[account]++
	}
	types := make([]string, 0, len(counts))
	for account := range counts {
		types = append(types, account)
	}
	slices.Sort(types)
	accounts := make([]DoctorAccountCount, 0, len(types))
	for _, account := range types {
		accounts = append(accounts, DoctorAccountCount{Type: account, Count: counts[account]})
	}
	return DoctorProfile{Status: diagnosticOK, Subscriptions: len(parsed.Subscriptions)}, accounts
}

func diagnoseLegacyFile(files fs.FS, directory, platform string) DoctorLegacyFile {
	info, err := fs.Stat(files, legacyTenantFilename)
	if errors.Is(err, fs.ErrNotExist) {
		return DoctorLegacyFile{Status: "absent"}
	}
	if err != nil || !info.Mode().IsRegular() {
		return DoctorLegacyFile{Status: diagnosticUnavailable}
	}
	name := path.Join(directory, legacyTenantFilename)
	quote := "'" + strings.ReplaceAll(name, "'", "'\"'\"'") + "'"
	command := "rm -- " + quote
	if platform == "windows" {
		name = filepath.Join(directory, legacyTenantFilename)
		command = "Remove-Item -LiteralPath '" + strings.ReplaceAll(name, "'", "''") + "'"
	}
	return DoctorLegacyFile{Status: "present", DeleteCommand: command}
}

func diagnoseSettings(files fs.FS, platform, azureVersion string) (DoctorSettings, string) {
	info := DoctorSettings{Status: diagnosticInvalid}
	settings, err := azcli.ReadSettings(files, nil)
	if err != nil {
		return info, ""
	}
	broker, brokerErr := azcli.BrokerStatus(settings, platform, azureVersion)
	if brokerErr == nil {
		info.Broker = &broker
	}
	login, loginErr := settings.Bool("core", "login_experience_v2", true)
	if loginErr == nil {
		info.LoginExperienceV2 = &login
	}
	if brokerErr == nil && loginErr == nil {
		info.Status = diagnosticOK
	}
	return info, settings.ActiveCloud()
}

func writeDoctorText(output io.Writer, report DoctorReport) error {
	var text strings.Builder
	_, _ = fmt.Fprintf(&text, "azctx %s\nUpdate: %s (cached, stale=%t, latest=%q; no network check)\nAzure CLI: %s, version=%q, path=%q\nConfig directory: %q\nProfile: %s (%d subscriptions)\nActive cloud: %q\nSettings: %s\n",
		report.AzctxVersion, report.Update.State, report.Update.Stale, report.Update.LatestVersion,
		report.AzureCLI.Status, report.AzureCLI.Version, report.AzureCLI.Path, report.ConfigDirectory,
		report.Profile.Status, report.Profile.Subscriptions, report.ActiveCloud, report.Settings.Status)
	if broker := report.Settings.Broker; broker != nil {
		if err := printBrokerStatus(&text, *broker, report.AzureCLI.Version); err != nil {
			return err
		}
	}
	if login := report.Settings.LoginExperienceV2; login != nil {
		_, _ = fmt.Fprintf(&text, "Login experience v2: %t (wrapped login uses child-only off)\n", *login)
	}
	for _, account := range report.AccountTypes {
		_, _ = fmt.Fprintf(&text, "Account type: %s (%d profile records)\n", account.Type, account.Count)
	}
	_, _ = fmt.Fprintf(&text, "Legacy tenant file: %s\n", report.LegacyTenantFile.Status)
	if report.LegacyTenantFile.DeleteCommand != "" {
		_, _ = fmt.Fprintf(&text, "Delete manually (quoted command): %q\n", report.LegacyTenantFile.DeleteCommand)
	}
	for _, warning := range report.Warnings {
		_, _ = fmt.Fprintf(&text, "Warning: %s\n", warning)
	}
	_, err := io.WriteString(output, text.String())
	return err
}
