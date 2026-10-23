package cmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/testutil"
)

const disabledStateTest = "Disabled"

func TestReadOnlyContextSelectionPolicy(t *testing.T) {
	for _, command := range []string{envCommand, execSubcommand} {
		for _, tc := range []struct {
			name, query, config string
			all                 bool
			code                int
		}{
			{"hidden alias", exportAliasName, "aliases:\n  prod: " + productionID + "\nhidden: [" + productionID + "]\n", false, 2},
			{"all exposes hidden alias", exportAliasName, "aliases:\n  prod: " + productionID + "\nhidden: [" + productionID + "]\n", true, 0},
			{"ambiguous even TTY", customerTenant, "", false, 2},
			{"alias name collision", productionName, "aliases:\n  Production: " + developmentID + "\n", false, 2},
			{"pins cannot resolve ambiguity", customerTenant, "pinned: [" + developmentID + "]\n", false, 2},
			{missingQuery, "nonexistent", "", false, 2},
			{"foreign cloud cannot be exposed", otherSubscriptionID, "", true, 2},
			{"disabled cannot be exposed", developmentID, "", true, 2},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				subs := tenantSubscriptions()
				subs = append(subs, profile.Subscription{Id: otherSubscriptionID, Name: "ForeignAccount", Tenant: tenantID, Environment: "OtherCloud"})
				if tc.name == "disabled cannot be exposed" {
					subs[1].State = disabledStateTest
				}
				fake := &testutil.FakeRunner{}
				deps := tenantDependencies(t, fake, subs)
				deps.Select = func([]profile.Subscription) (int, error) {
					t.Fatal("read-only context resolution must never open a picker")
					return -1, nil
				}
				deps.CheckUpdates = func(string, string) error {
					t.Fatal("read-only context resolution must never check updates")
					return nil
				}
				writeSelectionConfig(t, tc.config)
				args := []string{command, tc.query}
				if tc.all {
					args = append(args, metadataAllFlag)
				}
				wantCode := tc.code
				if command == execSubcommand {
					// A selected target reaches command resolution, not a switch.
					args = append(args, argumentDelimiter, "azctx-nonexistent-command")
					if wantCode == 0 {
						wantCode = 1
					}
				}
				var out, messages bytes.Buffer
				code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages)
				if code != wantCode || len(fake.Calls) != 0 {
					t.Fatalf("code=%d stdout=%q stderr=%q calls=%+v", code, &out, &messages, fake.Calls)
				}
				if tc.code == 0 && command == execSubcommand && !strings.Contains(messages.String(), "resolve command") {
					t.Fatalf("target did not reach exec: %q", &messages)
				}
			})
		}
	}
}

func TestCanceledEnvironmentGuardAndExecHaveNoSideEffects(t *testing.T) {
	for _, args := range [][]string{
		{envCommand},
		{envCommand, formatFlag, envGitHubFormat},
		{guardCommand, subscriptionFlag, productionID},
		{execSubcommand, productionID, argumentDelimiter, "not-run"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &testutil.FakeRunner{}
			deps := commandDependencies(t, fake)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			root := cmd.NewRootCommand(deps)
			root.SetContext(ctx)
			root.SetArgs(args)
			var out, messages bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&messages)
			if err := root.Execute(); cmd.ExitCode(err) != 130 || out.Len() != 0 || messages.Len() != 0 || len(fake.Calls) != 0 {
				t.Fatalf("error=%v stdout=%q stderr=%q calls=%+v", err, &out, &messages, fake.Calls)
			}
		})
	}
}

func TestExecSyntaxErrorsStayBeforeDelimiter(t *testing.T) {
	const notRunCommand = "command"
	for _, args := range [][]string{
		{execSubcommand},
		{execSubcommand, productionID},
		{execSubcommand, productionID, notRunCommand},
		{execSubcommand, productionID, argumentDelimiter},
		{execSubcommand, argumentDelimiter, notRunCommand},
		{execSubcommand, "", argumentDelimiter, notRunCommand},
		{execSubcommand, productionID, argumentDelimiter, ""},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			deps := commandDependencies(t, &testutil.FakeRunner{})
			args = append([]string{outputFlag, jsonFormat}, args...)
			var out, messages bytes.Buffer
			if code := cmd.Run(args, deps, strings.NewReader(""), &out, &messages); code != 1 || out.Len() != 0 ||
				!strings.HasPrefix(messages.String(), `{"code":"error"`) || strings.Count(messages.String(), "\n") != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &messages)
			}
		})
	}
}
