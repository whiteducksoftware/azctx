package utils

import (
	"os"

	"github.com/whiteducksoftware/azctx/log"

	"github.com/spf13/cobra"
)

// WrapCobraCommand wraps a cobra command with a function that may return error and logs the error
func WrapCobraCommandHandler(fun func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		err := fun(cmd, args)
		if err != nil {
			log.Error("%s", err)
			os.Exit(1)
		}
	}
}
