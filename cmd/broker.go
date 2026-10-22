package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/internal/azcli"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

type runnerFactory func() (azcli.Runner, error)

const (
	brokerActionStatus  = "status"
	brokerActionEnable  = "enable"
	brokerActionDisable = "disable"
)

func newAuthCommand(resolve runnerFactory, deps Dependencies) *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage Azure CLI authentication", Args: cobra.NoArgs}
	broker := &cobra.Command{Use: "broker", Short: "Inspect or explicitly change the persisted authentication broker", Args: cobra.NoArgs}
	for _, action := range []string{brokerActionStatus, brokerActionEnable, brokerActionDisable} {
		short := map[string]string{
			brokerActionStatus:  "Show effective broker settings and availability",
			brokerActionEnable:  "Enable the persisted authentication broker and sign in",
			brokerActionDisable: "Disable the persisted authentication broker and sign in",
		}[action]
		command := &cobra.Command{
			Use: action, Short: short, Args: cobra.NoArgs,
			RunE: commandHandler(func(cmd *cobra.Command, _ []string) error {
				return brokerRunE(cmd, action, resolve, deps)
			}),
		}
		if action != brokerActionStatus {
			command.Flags().Bool("yes", false, "Confirm account clear, persisted broker setting change and Azure sign-in")
		}
		broker.AddCommand(command)
	}
	auth.AddCommand(broker)
	return auth
}

func brokerRunE(command *cobra.Command, action string, resolve runnerFactory, deps Dependencies) error {
	if err := validateOutput(outputFormat(command), "auth broker "+action, formatText); err != nil {
		return err
	}
	files, _, err := azcli.OpenConfig(deps.FS, deps.ConfigDir)
	if err != nil {
		return err
	}
	settings, err := azcli.ReadSettings(files, nil)
	if err != nil {
		return err
	}
	info, err := azcli.BrokerStatus(settings, deps.Platform, "")
	if err != nil {
		return err
	}
	if !info.Supported && action != brokerActionStatus {
		return fmt.Errorf("authentication broker is not supported on %s", deps.Platform)
	}
	runner, err := resolve()
	if err != nil {
		return err
	}
	azureVersion, versionErr := azcli.CheckVersion(command.Context(), runner)
	if ExitCode(versionErr) == ExitInterrupted {
		return versionErr
	}
	if command.Context().Err() != nil {
		return command.Context().Err()
	}
	info, _ = azcli.BrokerStatus(settings, deps.Platform, azureVersion)
	if action == brokerActionStatus {
		if versionErr != nil && azureVersion == "" {
			return errors.New("could not determine Azure CLI version; broker availability is unknown")
		}
		return printBrokerStatus(command.OutOrStdout(), info, azureVersion)
	}
	if versionErr != nil {
		return versionErr
	}
	if !info.Available {
		return fmt.Errorf("authentication broker on %s requires Azure CLI %s or newer", deps.Platform, info.MinimumVersion)
	}
	return changeBroker(command, runner, info, action == brokerActionEnable, deps)
}

func printBrokerStatus(output io.Writer, info azcli.BrokerInfo, azureVersion string) error {
	if !info.Supported {
		_, err := fmt.Fprintf(output, "Authentication broker: unsupported on %s; use browser or device code sign-in.\n", info.Platform)
		return err
	}
	_, err := fmt.Fprintf(output, "Authentication broker: enabled=%t, available=%t (%s; Azure CLI %s; requires %s).\n", info.Enabled, info.Available, info.Setting, azureVersion, info.MinimumVersion)
	if err != nil {
		return err
	}
	if info.Preview {
		_, err = fmt.Fprintln(output, "macOS broker preview: requires an enrolled device with Company Portal and the Microsoft Enterprise SSO plug-in; availability does not verify enrollment.")
		if err != nil {
			return err
		}
	}
	return nil
}

func changeBroker(command *cobra.Command, runner azcli.Runner, info azcli.BrokerInfo, enabled bool, deps Dependencies) error {
	envKey := "AZURE_" + strings.ToUpper(strings.ReplaceAll(info.Setting, ".", "_"))
	if _, overridden := os.LookupEnv(envKey); overridden && info.Enabled != enabled {
		return fmt.Errorf("%s overrides the requested mode; unset it before changing the persisted broker setting", envKey)
	}
	if info.Preview {
		if _, err := fmt.Fprintln(command.ErrOrStderr(), "Warning: the macOS authentication broker is a preview and requires an enrolled device with Company Portal and the Microsoft Enterprise SSO plug-in."); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "This clears Azure CLI accounts, persists %s=%t, then starts az login. Cleared accounts cannot be restored by azctx.\n", info.Setting, enabled); err != nil {
		return err
	}
	confirmed, err := confirmBroker(command, deps)
	if err != nil || !confirmed {
		return err
	}
	info.Enabled = enabled
	if _, err := fmt.Fprintln(command.ErrOrStderr(), azcli.LoginPrompt(info, nil)); err != nil {
		return err
	}
	return azcli.SetBroker(command.Context(), runner, info.Platform, enabled, &azcli.Terminal{
		Stdin: command.InOrStdin(), Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
	})
}

func confirmBroker(command *cobra.Command, deps Dependencies) (bool, error) {
	if yes, _ := command.Flags().GetBool("yes"); yes {
		return true, nil
	}
	if !inputIsTerminal(command, deps) {
		return false, errors.New("broker changes require confirmation; use --yes to explicitly confirm without an azctx prompt")
	}
	if _, err := fmt.Fprint(command.ErrOrStderr(), "Continue? [y/N] "); err != nil {
		return false, err
	}
	answer, err := readConfirmation(command.Context(), command.InOrStdin())
	if err != nil {
		return false, fmt.Errorf("read broker confirmation (no changes made): %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(answer), "yes") && !strings.EqualFold(strings.TrimSpace(answer), "y") {
		_, _ = fmt.Fprintln(command.ErrOrStderr(), "Broker change canceled; no changes made.")
		return false, nil
	}
	return true, nil
}

// Do not buffer ahead or replace stdin: the attached az login needs the same
// terminal and any input following the confirmation line.
func readConfirmation(ctx context.Context, input io.Reader) (string, error) {
	var answer strings.Builder
	var char [1]byte
	for range 256 {
		if err := readPromptByte(ctx, input, char[:]); err != nil {
			return "", err
		}
		switch char[0] {
		case '\n':
			return answer.String(), nil
		case '\x03':
			return "", ui.ErrInterrupted
		case '\b', '\x7f':
			if answer.Len() > 0 {
				text := answer.String()
				answer.Reset()
				answer.WriteString(text[:len(text)-1])
			}
		default:
			answer.WriteByte(char[0])
		}
	}
	return "", errors.New("confirmation is too long")
}
