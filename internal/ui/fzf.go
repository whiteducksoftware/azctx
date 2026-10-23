package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

var ErrFZFUnavailable = errors.New("fzf is not on PATH")

// SelectFZF returns an index into the caller's ranked slice, never an ID/name.
// Only rows we supplied are accepted. Profile text is data, not shell/argv.
func SelectFZF(ctx context.Context, subscriptions []profile.Subscription, messages io.Writer) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	path, err := exec.LookPath("fzf")
	if err != nil {
		return -1, ErrFZFUnavailable
	}
	rows := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		rows[i] = strconv.Itoa(i) + "\t" + DisplayText(sub.DisplayName()) + "\t" + DisplayText(sub.Id) + "\t" + DisplayText(sub.TenantLabel()) + "\t" + DisplayText(sub.Tenant)
	}
	command := exec.CommandContext(ctx, path, fzfArguments(messages)...)
	// User defaults may contain execute bindings, preview commands or output flags.
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "FZF_") {
			command.Env = append(command.Env, variable)
		}
	}
	command.Stdin = strings.NewReader(strings.Join(rows, "\n") + "\n")
	command.Stderr = messages
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return -1, ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case 1:
				return -1, ErrCanceled
			case 130:
				return -1, ErrInterrupted
			}
		}
		return -1, fmt.Errorf("fzf selection failed: %w", err)
	}
	return parseFZFSelection(output.String(), rows)
}

func parseFZFSelection(output string, rows []string) (int, error) {
	key, selected, ok := strings.Cut(output, "\n")
	if key == "esc" {
		return -1, ErrCanceled
	}
	if ok && key == "" {
		selected = strings.TrimSuffix(selected, "\n")
		for i, row := range rows {
			if selected == row {
				return i, nil
			}
		}
	}
	return -1, errors.New("invalid fzf selection")
}

func fzfArguments(messages io.Writer) []string {
	// with-nth also transforms search fields; another nth would hide the name.
	args := []string{"--no-multi", "--no-sort", "--delimiter=\t", "--with-nth=2..", "--no-ansi", "--expect=esc", "--prompt=azctx> "}
	_, noColor := os.LookupEnv("NO_COLOR")
	file, ok := messages.(interface{ Fd() uintptr })
	if noColor || !ok || !term.IsTerminal(int(file.Fd())) {
		args = append(args, "--color=bw")
	}
	return args
}
