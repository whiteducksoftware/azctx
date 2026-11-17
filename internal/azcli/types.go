package azcli

import (
	"io/fs"

	"github.com/whiteducksoftware/azctx/internal/profile"
)

type CLI struct {
	fs          fs.FS
	runner      Runner
	configDir   string
	profile     profile.Profile
	activeCloud string
	activeIndex int
	// Terminal controls attached login streams; nil uses the process terminal.
	Terminal *Terminal

	// IncludeAll exposes hidden entries for listing, never for cloud switching.
	IncludeAll bool
}
