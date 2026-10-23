package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/whiteducksoftware/azctx/cmd"
)

func TestEveryCommandHasUsefulOfflineHelp(t *testing.T) {
	root := cmd.NewRootCommand(cmd.Dependencies{})
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		t.Run(command.CommandPath(), func(t *testing.T) {
			if command.Short == "" || len(command.Long) <= len(command.Short) {
				t.Errorf("missing distinct Short and explanatory Long help")
			}
			examples := strings.Split(strings.TrimSpace(command.Example), "\n")
			if len(examples) < 2 || len(examples) > 3 {
				t.Fatalf("want two or three real examples, got %d", len(examples))
			}
			for _, example := range examples {
				if !strings.HasPrefix(strings.TrimSpace(example), "azctx") {
					t.Fatalf("not an azctx invocation: %q", example)
				}
				for _, args := range expandAgentSynopsis(example) {
					if err := parseAgentExample(args, true); err != nil {
						t.Errorf("invalid help example %q: %v", example, err)
					}
				}
			}
			var output bytes.Buffer
			help := cmd.NewRootCommand(cmd.Dependencies{})
			help.SetOut(&output)
			help.SetErr(&output)
			help.SetArgs(append(strings.Fields(strings.TrimPrefix(command.CommandPath(), "azctx")), "--help"))
			if err := help.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), strings.TrimSpace(command.Example)) {
				t.Fatal("rendered --help lost examples")
			}
		})
		for _, child := range command.Commands() {
			if !child.Hidden {
				visit(child)
			}
		}
	}
	visit(root)
}
