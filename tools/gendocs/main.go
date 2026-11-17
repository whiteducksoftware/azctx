// Command gendocs writes or checks the deterministic CLI reference.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/whiteducksoftware/azctx/internal/clidocs"
)

func main() {
	check := flag.Bool("check", false, "Check documentation without changing files")
	directory := flag.String("dir", "docs/cli", "Generated reference directory")
	flag.Parse()
	if err := clidocs.Sync(*directory, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
