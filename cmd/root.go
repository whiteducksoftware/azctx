package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/config"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/selection"
	"github.com/whiteducksoftware/azctx/internal/state"
	"github.com/whiteducksoftware/azctx/internal/ui"
	"github.com/whiteducksoftware/azctx/internal/update"
	"github.com/whiteducksoftware/azctx/log"
)

const (
	// Repository Owner
	owner = "whiteducksoftware"

	// Repository Name
	repo = "azctx"
)

// Dependencies supplies the external boundaries used by the command tree.
// Nil fields use production implementations, resolved lazily where needed.
type Dependencies struct {
	Runner       azcli.Runner
	FS           fs.FS // Read-only filesystem rooted at the Azure configuration directory.
	ConfigDir    string
	Select       func([]profile.Subscription) (int, error)
	CheckUpdates func(string, string) error
	StdinTTY     func() bool // Nil probes Cobra stdin; input opt-outs still take precedence.
	StdoutTTY    func() bool // Nil probes Cobra stdout for optional update checks.
	Platform     string      // Empty uses runtime.GOOS; tests can exercise broker capabilities.
}

// NewRootCommand constructs an independent command tree, including flag state.
func NewRootCommand(deps Dependencies) *cobra.Command {
	if deps.Platform == "" {
		deps.Platform = runtime.GOOS
	}
	verbose := false
	resolveRunner := func() (azcli.Runner, error) {
		if deps.Runner == nil {
			runner, err := azcli.NewRunner()
			if err != nil {
				return nil, fmt.Errorf("azure cli is not installed. please install it and try again. See here: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli: %w", err)
			}
			deps.Runner = runner
		}
		return deps.Runner, nil
	}
	newCLI := func(forLogin bool) (azcli.CLI, error) {
		runner, err := resolveRunner()
		if err != nil {
			return azcli.CLI{}, err
		}
		if forLogin {
			return azcli.NewForLogin(deps.FS, runner, deps.ConfigDir)
		}
		return azcli.New(deps.FS, runner, deps.ConfigDir)
	}
	loadCLI := func() (azcli.CLI, error) { return newCLI(false) }
	rootCmd := &cobra.Command{
		Use:   "azctx [query | -]",
		Short: "azctx is a CLI tool for managing azure cli subscriptions",
		Long: `azctx is a CLI tool for managing azure cli subscriptions.
	It is a helper for the azure cli and provides a simple interface for managing subscriptions.
	Pass a subscription name, tenant/subscription or subscription ID to switch.
	Use -- before names that equal a command. Pass - for the previous subscription.
	Without interactive input, the bare command lists subscriptions.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		PersistentPreRun: func(command *cobra.Command, _ []string) {
			output := command.ErrOrStderr()
			if outputFormat(command) == formatJSON {
				output = io.Discard
			}
			log.InitLogger(output)
			log.SetVerbose(verbose && outputFormat(command) != formatJSON)
		},
		RunE: commandHandler(func(cmd *cobra.Command, args []string) error {
			return rootRunE(cmd, args, loadCLI, deps)
		}),
	}
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug diagnostics")
	rootCmd.PersistentFlags().Bool("no-input", false, "Never open a picker or prompt (also AZCTX_NO_INPUT=1)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (depends on command)")
	rootCmd.PersistentFlags().Bool("all", false, "Expose configured hidden subscriptions; list also includes disabled, tenant-level and other-cloud accounts")

	rootCmd.Flags().BoolP("current", "c", false, "Display the current active subscription")
	rootCmd.Flags().Bool("yes", false, "Confirm switching to a protected subscription without prompting")
	rootCmd.ValidArgsFunction = completeSubscriptions(deps)
	rootCmd.AddCommand(newListCommand(loadCLI))
	rootCmd.AddCommand(newTenantCommand(loadCLI, deps))
	rootCmd.AddCommand(newHistoryCommand(deps))
	rootCmd.AddCommand(newEnvCommand(deps))
	rootCmd.AddCommand(newGuardCommand(deps))
	rootCmd.AddCommand(newExecCommand(deps))
	rootCmd.AddCommand(newConfigCommand(deps))
	rootCmd.AddCommand(newAliasCommand(deps))
	rootCmd.AddCommand(newLoginCommand(func() (azcli.CLI, error) { return newCLI(true) }, deps))
	rootCmd.AddCommand(newAuthCommand(resolveRunner, deps))
	rootCmd.AddCommand(newDoctorCommand(resolveRunner, deps))
	configureHelp(rootCmd)
	return rootCmd
}

func Execute() error {
	command, err := NewRootCommand(Dependencies{}).ExecuteC()
	// Cobra adds this hint only when command discovery fails, before invocation.
	if err != nil && command != nil && command.CalledAs() == "" {
		return fmt.Errorf("%w\nRun '%s --help' for usage.", err, command.CommandPath()) //nolint:revive // Preserve the public multiline command-discovery diagnostic.
	}
	return err
}

func rootRunE(cmd *cobra.Command, args []string, newCLI func() (azcli.CLI, error), deps Dependencies) error {
	current, _ := cmd.Flags().GetBool("current")
	if current && len(args) != 0 {
		return errors.New("--current does not accept a query")
	}
	if err := validateRootOutput(cmd, current, len(args) != 0); err != nil {
		return err
	}
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	// Initialize the CLI
	cli, err := newCLI()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cli.ConfigDir())
	if err != nil {
		return err
	}
	all, _ := cmd.Flags().GetBool("all")
	if current {
		return getActiveSubscription(cmd, cli, cfg)
	}
	if len(args) == 0 && (outputFormat(cmd) != "" || !inputIsTerminal(cmd, deps)) {
		cli.IncludeAll = all
		return listSubscriptions(cmd, cli)
	}
	recent, err := state.Recent(cli.ConfigDir())
	if err != nil {
		return err
	}
	catalog := selection.Catalog{Subscriptions: cli.Subscriptions(), Config: cfg, Recent: recent}
	candidates, err := switchCandidates(cmd, args, cli, deps, catalog, all)
	if err != nil {
		return err
	}
	target, err := pickSubscription(args, candidates, rankedSelector(cmd, deps, catalog))
	if err != nil {
		return err
	}
	cli.Context = cmd.Context()
	return switchConfigured(cmd, cli, deps, cfg, target)
}

func rankedSelector(cmd *cobra.Command, deps Dependencies, catalog selection.Catalog) func([]profile.Subscription) (int, error) {
	return func(subscriptions []profile.Subscription) (int, error) {
		legacyProfileHint(cmd, subscriptions)
		if deps.Select == nil {
			ui.SortSubscriptions(subscriptions)
		}
		catalog.Options().Rank(subscriptions)
		return configuredPicker(cmd, deps, catalog.Config, subscriptions, false)
	}
}

func configuredPicker(command *cobra.Command, deps Dependencies, cfg config.Config, rows []profile.Subscription, tenants bool) (int, error) {
	if err := command.Context().Err(); err != nil {
		return -1, err
	}
	if cfg.Picker == "fzf" {
		index, err := ui.SelectFZF(command.Context(), rows, command.ErrOrStderr())
		if !errors.Is(err, ui.ErrFZFUnavailable) {
			return index, err
		}
		const notice = "azctx.fzf.fallback"
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		if command.Annotations[notice] == "" {
			_, _ = fmt.Fprintln(command.ErrOrStderr(), "fzf is not on PATH; using the built-in picker.")
			command.Annotations[notice] = "shown"
		}
	}
	if deps.Select != nil {
		return deps.Select(rows)
	}
	if tenants {
		return ui.SelectTenantsOrdered(rows)
	}
	return ui.SelectOrdered(rows)
}

func switchCandidates(cmd *cobra.Command, args []string, cli azcli.CLI, deps Dependencies, catalog selection.Catalog, all bool) ([]profile.Subscription, error) {
	if len(args) == 0 {
		return catalog.Visible(all), nil
	}
	var candidates []profile.Subscription
	if args[0] == "-" {
		id, err := state.Load(cli.ConfigDir())
		if err != nil {
			return nil, err
		}
		for _, sub := range catalog.Visible(all) {
			if strings.EqualFold(sub.Id, id) {
				candidates = append(candidates, sub)
			}
		}
		if len(candidates) == 0 {
			return nil, &ExitError{Code: CodeNoMatch, Message: fmt.Sprintf("previous subscription %s is no longer available", id), Status: ExitNoMatch}
		}
	} else {
		var err error
		candidates, err = catalog.Find(args[0], all)
		if err != nil {
			return nil, &ExitError{Code: CodeNoMatch, Message: err.Error(), Status: ExitNoMatch, Cause: err}
		}
	}
	if len(candidates) > 1 && (!inputIsTerminal(cmd, deps) || outputFormat(cmd) == formatJSON) {
		return nil, &ExitError{
			Code: CodeAmbiguousMatch, Message: fmt.Sprintf("multiple subscriptions match %q; use a subscription ID or tenant/subscription", args[0]),
			Status: ExitNoMatch, Candidates: SubscriptionOutputs(candidates),
		}
	}
	return candidates, nil
}

func pickSubscription(args []string, candidates []profile.Subscription, selectItem func([]profile.Subscription) (int, error)) (profile.Subscription, error) {
	if len(candidates) == 0 {
		return profile.Subscription{}, &ExitError{Code: CodeNoMatch, Message: "no subscription found", Status: ExitNoMatch}
	}
	index := 0
	if len(args) == 0 || len(candidates) > 1 {
		var err error
		index, err = selectItem(candidates)
		if err != nil {
			return profile.Subscription{}, err
		}
		if index < 0 || index >= len(candidates) {
			return profile.Subscription{}, errors.New("invalid picker selection")
		}
	}
	return candidates[index], nil
}

func switchConfigured(cmd *cobra.Command, cli azcli.CLI, deps Dependencies, cfg config.Config, target profile.Subscription) error {
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	if config.Contains(cfg.Protected, target) {
		if err := confirmProtected(cmd, deps, target); err != nil {
			return err
		}
	}
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	if err := switchSubscription(cmd, cli, deps, target); err != nil {
		return err
	}

	if cfg.UpdateCheck {
		checkForUpdates(cmd, deps, cli.ConfigDir())
	}
	return nil
}

func checkForUpdates(cmd *cobra.Command, deps Dependencies, azureDir string) {
	_, ci := os.LookupEnv("CI")
	if !outputIsTerminal(cmd, deps) || ci || os.Getenv("AZCTX_NO_UPDATE_CHECK") == "1" || outputFormat(cmd) == formatJSON {
		return
	}
	check := deps.CheckUpdates
	if check == nil {
		check = (update.Checker{
			AzureDir: azureDir, Output: cmd.ErrOrStderr(),
			IsTerminal: func() bool { return outputIsTerminal(cmd, deps) },
		}).Check
	}
	if err := check(owner, repo); err != nil {
		log.Debug("Update check failed: %v", err)
	}
}

func streamIsTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

func outputIsTerminal(command *cobra.Command, deps Dependencies) bool {
	if deps.StdoutTTY != nil {
		return deps.StdoutTTY()
	}
	return streamIsTerminal(command.OutOrStdout())
}

func inputIsTerminal(command *cobra.Command, deps Dependencies) bool {
	noInput, _ := command.Flags().GetBool("no-input")
	_, ci := os.LookupEnv("CI")
	if noInput || os.Getenv("AZCTX_NO_INPUT") == "1" || ci {
		return false
	}
	return stdinIsTerminal(command, deps)
}

func stdinIsTerminal(command *cobra.Command, deps Dependencies) bool {
	if deps.StdinTTY != nil {
		return deps.StdinTTY()
	}
	return streamIsTerminal(command.InOrStdin())
}

// switchSubscription holds the retained state lock from reading state through
// az account set, any rollback and best-effort history, so concurrent switches
// neither lose previous/MRU updates nor roll back over a newer commit. Waiting
// for the lock honors cancellation; history.lock is only ever taken inside it.
// The local Azure profile is re-read under the lock (no az call), so previous in
// state and history is the context a preceding switch actually left active.
func switchSubscription(command *cobra.Command, cli azcli.CLI, deps Dependencies, subscription profile.Subscription) error {
	if err := profile.ValidateGUID(subscription.Id); err != nil {
		return err
	}
	tx, err := state.Begin(command.Context(), cli.ConfigDir())
	if err != nil {
		return err
	}
	defer tx.Release()
	if err := cli.Reload(); err != nil {
		return fmt.Errorf("reload Azure profile before switching (no changes made): %w", err)
	}
	current, err := cli.ActiveSubscription()
	previous := ""
	if err == nil && !strings.EqualFold(current.Id, subscription.Id) {
		previous = current.Id
	}
	rollback, err := tx.RecordSelection(previous, subscription.Id)
	if err != nil {
		return fmt.Errorf("save previous subscription: %w", err)
	}
	log.Debug("Setting active subscription to %s/%s (%s/%s)",
		ui.DisplayText(subscription.TenantLabel()), ui.DisplayText(subscription.Name),
		ui.DisplayText(subscription.Tenant), ui.DisplayText(subscription.Id))
	if err := cli.SetSubscription(subscription); err != nil {
		if restoreErr := rollback(); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous subscription state: %w", restoreErr))
		}
		return err
	}
	recordHistory(command, cli, deps, subscription)
	return nil
}

func getActiveSubscription(command *cobra.Command, cli azcli.CLI, cfg config.Config) error {
	// Try to get the active subscription
	subscription, err := cli.ActiveSubscription()
	if err != nil {
		return err
	}

	if err := writeCurrent(command, subscription, config.Contains(cfg.Protected, subscription)); err != nil {
		return err
	}

	warnProjectContext(command, subscription)
	legacyProfileHint(command, []profile.Subscription{subscription})
	return nil
}

// confirmProtected returns nil only for an explicit confirmation. A declined or
// default answer is a classified failure, so `azctx prod && cmd` cannot proceed.
func confirmProtected(command *cobra.Command, deps Dependencies, sub profile.Subscription) error {
	if yes, _ := command.Flags().GetBool("yes"); yes {
		return nil
	}
	if !inputIsTerminal(command, deps) || outputFormat(command) == formatJSON {
		return errors.New("protected subscription requires confirmation; use --yes to explicitly confirm without an azctx prompt")
	}
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "Switch to protected subscription %s/%s (%s)? [y/N] ",
		ui.DisplayText(sub.TenantLabel()), ui.DisplayText(sub.Name), ui.DisplayText(sub.Id)); err != nil {
		return err
	}
	answer, err := readConfirmation(command.Context(), command.InOrStdin())
	if err != nil {
		return fmt.Errorf("read protected subscription confirmation (no changes made): %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(answer), "y") && !strings.EqualFold(strings.TrimSpace(answer), "yes") {
		return &ExitError{Code: CodeError, Message: "switch canceled; no changes made", Status: ExitFailure}
	}
	return nil
}
