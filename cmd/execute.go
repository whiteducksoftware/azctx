package cmd

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
)

// Run executes the complete CLI boundary, including Cobra parse-error presentation.
func Run(args []string, deps Dependencies, stdin io.Reader, stdout, stderr io.Writer) int {
	command := NewRootCommand(deps)
	command.SetIn(stdin)
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetArgs(args)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := command.ExecuteContext(ctx)
	return WriteError(command.ErrOrStderr(), requestedOutput(args), err)
}

// Inspect flags independently of Cobra: parsing may stop before the output flag.
// The delimiter ends azctx flags, including arguments forwarded to az login.
func requestedOutput(args []string) string {
	format := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		switch {
		case arg == "-o" || arg == "--output":
			if i+1 < len(args) {
				i++
				format = args[i]
			}
		case strings.HasPrefix(arg, "--output="):
			format = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--"):
			if value, found := shortOutput(arg[1:]); found {
				if value == "" && i+1 < len(args) {
					i++
					value = args[i]
				} else {
					value = strings.TrimPrefix(value, "=")
				}
				format = value
			}
		}
	}
	return format
}

func shortOutput(flags string) (string, bool) {
	for i, flag := range flags {
		switch flag {
		case 'c', 'h':
			// Cobra permits grouping boolean shorthands before -o.
		case 'o':
			return flags[i+1:], true
		default:
			return "", false
		}
	}
	return "", false
}
