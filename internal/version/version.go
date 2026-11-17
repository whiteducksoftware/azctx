// Package version reports release ldflags with Go build metadata as a fallback.
package version

import (
	"runtime"
	"runtime/debug"
)

// These variables are populated by release builds with -ldflags -X.
var (
	Version   string
	BuildDate string
	Commit    string
)

type Info struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
}

func Get() Info {
	info := Info{Version: Version, BuildDate: BuildDate, Commit: Commit, GoVersion: runtime.Version()}
	if build, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" && build.Main.Version != "" {
			info.Version = build.Main.Version
		}
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" && info.Commit == "" {
				info.Commit = setting.Value
			}
			if setting.Key == "vcs.time" && info.BuildDate == "" {
				info.BuildDate = setting.Value
			}
		}
	}
	if info.Version == "" {
		info.Version = "(devel)"
	}
	if info.BuildDate == "" {
		info.BuildDate = "unknown"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	return info
}
