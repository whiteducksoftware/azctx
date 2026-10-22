package azcli

import (
	"context"
	"slices"
)

// WithConfigDir binds an explicit Azure directory to every child invocation.
// An empty directory leaves ordinary inherited configuration unchanged.
func WithConfigDir(runner Runner, configDir string) Runner {
	if configDir == "" {
		return runner
	}
	return configRunner{Runner: runner, configDir: configDir}
}

type configRunner struct {
	Runner
	configDir string
}

func (r configRunner) Run(ctx context.Context, args []string, options Options) (Result, error) {
	options.Env = append(slices.Clone(options.Env), configDirEnv+"="+r.configDir)
	return r.Runner.Run(ctx, args, options)
}
