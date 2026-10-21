// Package azcli delegates Azure operations to an injected Runner and reads the
// Azure CLI configuration without handling credentials.
package azcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/log"
)

// New loads the profile and Azure CLI settings using an already resolved runner.
// Files is read-only and rooted at configDir. With nil files, an empty configDir
// selects AZURE_CONFIG_DIR or the user's .azure directory.
func New(files fs.FS, runner Runner, configDir string) (CLI, error) {
	return newCLI(files, runner, configDir, false)
}

// NewForLogin allows a missing Azure directory/profile so Azure CLI can create
// them. Existing files are still read-only, and malformed settings remain errors.
func NewForLogin(files fs.FS, runner Runner, configDir string) (CLI, error) {
	return newCLI(files, runner, configDir, true)
}

func newCLI(files fs.FS, runner Runner, configDir string, allowMissing bool) (CLI, error) {
	if files == nil {
		var err error
		configDir, err = resolveConfigDir(configDir, allowMissing)
		if err != nil {
			return CLI{}, err
		}
		files = os.DirFS(configDir)
	}
	cli := CLI{
		fs: files, runner: runner, configDir: configDir, activeIndex: -1,
	}

	// Refresh the CLI instance
	err := cli.reload(allowMissing)
	if err != nil {
		return CLI{}, err
	}

	return cli, nil
}

// Reload refreshes the CLI instance by fetching the latest data from the azure cli config files
func (cli *CLI) Reload() error {
	return cli.reload(false)
}

func (cli *CLI) reload(allowMissing bool) error {
	settings, err := ReadSettings(cli.fs, nil)
	if err != nil {
		return err
	}
	clouds, err := readINI(cli.fs, "clouds.config")
	if err != nil {
		return err
	}
	if err := cli.readProfile(); err != nil {
		if !allowMissing || !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		cli.profile = profile.Profile{}
	}
	cli.activeCloud = settings.ActiveCloud()
	cli.settings = settings
	remembered, _ := iniValue(clouds, cli.activeCloud, "subscription")
	return cli.resolveDefault(remembered)
}

// InteractiveLogin executes the az login command
func (cli CLI) InteractiveLogin(extraArgs []string) error {
	if err := validateLoginIDs(extraArgs); err != nil {
		return err
	}
	prompt, err := cli.loginPrompt(extraArgs)
	if err != nil {
		return err
	}
	log.Info("Logging in... %s", prompt)

	// Execute the az login command
	err = cli.execLogin(extraArgs)
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	return nil
}

// IterativeTenantLogin explicitly executes the az login command for each tenant
func (cli CLI) IterativeTenantLogin(extraArgs []string) error {
	if err := validateLoginIDs(extraArgs); err != nil {
		return err
	}
	for _, tenant := range cli.tenantSubscriptions() {
		if err := profile.ValidateGUID(tenant.Tenant); err != nil {
			return fmt.Errorf("invalid tenant: %w", err)
		}
	}
	prompt, err := cli.loginPrompt(extraArgs)
	if err != nil {
		return err
	}
	// Loop through the tenants and execute the az login command
	for _, tenant := range cli.tenantSubscriptions() {
		log.Info("Logging into tenant %s... %s", tenant.TenantLabel(), prompt)

		// Execute the az login command
		args := []string{"--tenant", tenant.Tenant}
		args = append(args, extraArgs...)
		err := cli.execLogin(args)
		if err != nil {
			if cli.context().Err() != nil {
				return cli.context().Err()
			}
			log.Error("Failed to login to tenant %s. Azure CLI returned the following error: %s", tenant.TenantLabel(), err.Error())
			continue
		}
	}

	return nil
}

func (cli CLI) loginPrompt(args []string) (string, error) {
	azureVersion, err := CheckVersion(cli.context(), cli.runner)
	if err != nil {
		return "", err
	}
	platform := cli.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	info, err := BrokerStatus(cli.settings, platform, azureVersion)
	if err != nil {
		return "", err
	}
	return LoginPrompt(info, args), nil
}

// SetSubscription asks az to set the default subscription.
func (cli CLI) SetSubscription(subscription profile.Subscription) error {
	if err := profile.ValidateGUID(subscription.Id); err != nil {
		return fmt.Errorf("invalid subscription: %w", err)
	}
	if subscription.Environment != cli.activeCloud {
		return fmt.Errorf("subscription %s is not in active cloud %s", subscription.Id, cli.activeCloud)
	}
	// Execute az account set command
	args := []string{"account", "set", "--subscription", subscription.Id}
	return cli.capture(args)
}

// ConfigDir is the protected, read-only Azure configuration directory.
func (cli CLI) ConfigDir() string { return cli.configDir }

func (cli CLI) tenantSubscriptions() []profile.Subscription {
	var tenants []profile.Subscription
	seen := make(map[string]bool)
	for _, sub := range cli.profile.Subscriptions {
		id := strings.ToLower(sub.Tenant)
		if sub.Environment == cli.activeCloud && id != "" && !seen[id] {
			seen[id] = true
			tenants = append(tenants, sub)
		}
	}
	return tenants
}

// TenantCount counts distinct token tenants in the active cloud profile.
func (cli CLI) TenantCount() int { return len(cli.tenantSubscriptions()) }

// LegacyProfile reports missing tenant metadata in the visible profile.
func (cli CLI) LegacyProfile() bool {
	return profile.HasLegacyTenantMetadata(cli.Subscriptions())
}
