package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/whiteducksoftware/azctx/cmd"
	"github.com/whiteducksoftware/azctx/log"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var handlerErr *cmd.HandlerError
		if errors.As(err, &handlerErr) {
			log.Error("%s", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
		os.Exit(1)
	}
}
