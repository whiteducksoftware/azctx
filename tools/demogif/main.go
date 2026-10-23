// Command demogif writes or checks the README's synthetic picker animation.
//
// Every terminal frame is the View content of the real ui.Picker after real key
// messages. Identities are synthetic; no Azure CLI, profile or account is used.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
)

const defaultOutput = "assets/img/azctx-demo.gif"

func main() {
	check := flag.Bool("check", false, "Check the GIF without changing files")
	output := flag.String("o", defaultOutput, "GIF path, relative to the current directory (run from the repository root)")
	flag.Parse()
	if err := writeOrCheck(*output, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeOrCheck(path string, check bool) error {
	data, err := generate()
	if err != nil {
		return err
	}
	return writeOrCheckData(path, data, check)
}

func writeOrCheckData(path string, data []byte, check bool) error {
	if !check {
		return os.WriteFile(path, data, 0o644) //nolint:gosec // A committed README image must be world-readable.
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, data) {
		return errors.New(path + " differs from the generated demo")
	}
	return nil
}
