package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/profile"
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
}

// NewRootCommand constructs an independent command tree, including flag state.
func NewRootCommand(deps Dependencies) *cobra.Command {
	if deps.Select == nil {
		deps.Select = ui.Select
	}
	verbose := false
	newCLI := func() (azcli.CLI, error) {
		if deps.Runner == nil {
			runner, err := azcli.NewRunner()
			if err != nil {
				return azcli.CLI{}, fmt.Errorf("azure cli is not installed. please install it and try again. See here: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli: %w", err)
			}
			deps.Runner = runner
		}
		return azcli.New(deps.FS, deps.Runner, deps.ConfigDir)
	}
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
			return rootRunE(cmd, args, newCLI, deps)
		}),
	}
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug diagnostics")
	rootCmd.PersistentFlags().Bool("no-input", false, "Never open a picker or prompt (also AZCTX_NO_INPUT=1)")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (depends on command)")

	rootCmd.Flags().BoolP("current", "c", false, "Display the current active subscription")
	rootCmd.AddCommand(newListCommand(newCLI))
	rootCmd.AddCommand(newLoginCommand(newCLI))
	return rootCmd
}

func Execute() error {
	command, err := NewRootCommand(Dependencies{}).ExecuteC()
	// Cobra adds this hint only when command discovery fails, before invocation.
	if err != nil && command != nil && command.CalledAs() == "" {
		return fmt.Errorf("%w\nRun '%s --help' for usage.", err, command.CommandPath())
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
	// Initialize the CLI
	cli, err := newCLI()
	if err != nil {
		return err
	}
	selectItem := func(subscriptions []profile.Subscription) (int, error) {
		legacyProfileHint(cmd, cli)
		return deps.Select(subscriptions)
	}

	// check the flags
	switch {
	case current:
		return getActiveSubscription(cmd, cli)
	case len(args) == 1:
		// check if the user passed -
		if args[0] == "-" {
			err = selectPreviousSubscription(cli)
		} else {
			err = selectSubscriptionByName(cli, args[0], selectItem, inputIsTerminal(cmd, deps) && outputFormat(cmd) != formatJSON)
		}
		if err != nil {
			return err
		}
	default:
		if outputFormat(cmd) != "" || !inputIsTerminal(cmd, deps) {
			return listSubscriptions(cmd, cli)
		}
		err = interactivelySelectSubscription(cli, selectItem)
		if err != nil {
			return err
		}
	}

	checkForUpdates(cmd, deps, cli.ConfigDir())
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
	if deps.StdinTTY != nil {
		return deps.StdinTTY()
	}
	return streamIsTerminal(command.InOrStdin())
}

func interactivelySelectSubscription(cli azcli.CLI, selectItem func([]profile.Subscription) (int, error)) error {
	// Ask the user to select a subscription
	subscriptions := cli.Subscriptions()

	// Run the prompt
	idx, err := selectItem(subscriptions)
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(subscriptions) {
		return errors.New("invalid picker selection")
	}
	return switchSubscription(cli, subscriptions[idx])
}

func selectSubscriptionByName(cli azcli.CLI, name string, selectItem func([]profile.Subscription) (int, error), interactive bool) error {
	subscriptions, err := cli.TryFindSubscription(name)
	if err != nil {
		return &ExitError{Code: CodeNoMatch, Message: err.Error(), Status: ExitNoMatch, Cause: err}
	}

	// Check if we found more than one subscription
	var subscription profile.Subscription
	switch length := len(subscriptions); {
	case length > 1:
		if !interactive {
			return &ExitError{
				Code: CodeAmbiguousMatch, Message: fmt.Sprintf("multiple subscriptions match %q; use a subscription ID or tenant/subscription", name),
				Status: ExitNoMatch, Candidates: SubscriptionOutputs(subscriptions),
			}
		}
		// Run the prompt
		idx, err := selectItem(subscriptions)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= len(subscriptions) {
			return errors.New("invalid picker selection")
		}

		subscription = subscriptions[idx]
	case length == 1:
		subscription = subscriptions[0]
	default:
		return &ExitError{Code: CodeNoMatch, Message: "no subscription found", Status: ExitNoMatch}
	}

	// Set the selected subscription as the default
	return switchSubscription(cli, subscription)
}

func selectPreviousSubscription(cli azcli.CLI) error {
	id, err := state.Load(cli.ConfigDir())
	if err != nil {
		return err
	}
	for _, subscription := range cli.Subscriptions() {
		if strings.EqualFold(subscription.Id, id) {
			return switchSubscription(cli, subscription)
		}
	}
	return &ExitError{Code: CodeNoMatch, Message: fmt.Sprintf("previous subscription %s is no longer available", id), Status: ExitNoMatch}
}

func switchSubscription(cli azcli.CLI, subscription profile.Subscription) error {
	if err := profile.ValidateGUID(subscription.Id); err != nil {
		return err
	}
	current, err := cli.ActiveSubscription()
	var rollback func() error
	if err == nil && !strings.EqualFold(current.Id, subscription.Id) {
		rollback, err = state.Record(cli.ConfigDir(), current.Id)
		if err != nil {
			return fmt.Errorf("save previous subscription: %w", err)
		}
	}
	log.Debug("Setting active subscription to %s/%s (%s/%s)", subscription.TenantLabel(), subscription.Name, subscription.Tenant, subscription.Id)
	err = cli.SetSubscription(subscription)
	if err != nil && rollback != nil {
		if restoreErr := rollback(); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous subscription state: %w", restoreErr))
		}
	}
	return err
}

func getActiveSubscription(command *cobra.Command, cli azcli.CLI) error {
	// Try to get the active subscription
	subscription, err := cli.ActiveSubscription()
	if err != nil {
		return err
	}

	if err := writeCurrent(command, subscription); err != nil {
		return err
	}
	legacyProfileHint(command, cli)
	return nil
}
