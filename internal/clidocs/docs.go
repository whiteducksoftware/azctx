// Package clidocs renders and checks the public CLI reference without Azure IO.
package clidocs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/whiteducksoftware/azctx/cmd"
)

// Sync writes deterministic Cobra Markdown, or checks exact contents and file set.
// The directory is owned exclusively by the generated CLI reference.
func Sync(directory string, check bool) error {
	files := make(map[string][]byte)
	if err := render(cmd.NewRootCommand(cmd.Dependencies{}), files); err != nil {
		return err
	}
	if !check {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := filepath.Join(directory, name)
		if check {
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, files[name]) {
				return fmt.Errorf("stale CLI documentation: %s; run make docs", path)
			}
		} else if err := os.WriteFile(path, files[name], 0o600); err != nil {
			return err
		}
	}
	return checkExtraFiles(directory, files, check)
}

func render(command *cobra.Command, files map[string][]byte) error {
	command.DisableAutoGenTag = true
	// Cobra's renderer adds local help commands. Walk the real tree first so
	// those renderer-only children never become fictitious CLI reference pages.
	for _, child := range command.Commands() {
		if !child.Hidden {
			if err := render(child, files); err != nil {
				return err
			}
		}
	}
	var output bytes.Buffer
	if err := doc.GenMarkdown(command, &output); err != nil {
		return err
	}
	data := bytes.ReplaceAll(output.Bytes(), []byte("\t - "), []byte("\t: "))
	files[strings.ReplaceAll(command.CommandPath(), " ", "_")+".md"] = append(bytes.TrimRight(data, "\n"), '\n')
	return nil
}

func checkExtraFiles(directory string, files map[string][]byte, check bool) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, exists := files[entry.Name()]; exists {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if check || entry.IsDir() {
			return fmt.Errorf("unexpected CLI documentation: %s; run make docs", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
