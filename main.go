package main

import (
	"os"

	"github.com/whiteducksoftware/azctx/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:], cmd.Dependencies{}, os.Stdin, os.Stdout, os.Stderr))
}
