package azcli

import (
	"context"
	"io/fs"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

type CLI struct {
	fs          fs.FS
	runner      Runner
	configDir   string
	profile     profile.Profile
	settings    Settings
	activeCloud string
	activeIndex int
	// Terminal controls attached login streams; nil uses the process terminal.
	Terminal *Terminal
	// Context and Platform are optional login execution boundaries.
	Context  context.Context
	Platform string
	// LoginPassthrough permits only the user's explicit argv after --. IDs
	// derived by azctx must still be validated before adding them to that argv.
	LoginPassthrough bool

	// IncludeAll exposes hidden entries for listing, never for cloud switching.
	IncludeAll bool
}
