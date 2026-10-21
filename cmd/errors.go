package cmd

import "github.com/spf13/cobra"

// HandlerError distinguishes logged handler failures from Cobra's argument
// errors, preserving their existing presentation without exiting in a handler.
type HandlerError struct {
	err error
}

func (e *HandlerError) Error() string { return e.err.Error() }
func (e *HandlerError) Unwrap() error { return e.err }

func commandHandler(handler func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		if err := handler(command, args); err != nil {
			return &HandlerError{err: err}
		}
		return nil
	}
}
