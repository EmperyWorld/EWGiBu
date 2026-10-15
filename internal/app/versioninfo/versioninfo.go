// Package versioninfo exposes product identity for CLI and future API adapters.
package versioninfo

import (
	"runtime"

	"iacpack/internal/buildinfo"
)

// Info is the deterministic version payload returned by the version command.
type Info struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	Commit    string `json:"commit"`
	Dirty     bool   `json:"dirty"`
}

// Current returns the version identity of this process.
// It does not consult the clock, hostname, or working directory.
func Current() Info {
	return Info{
		Product:   buildinfo.Product,
		Version:   buildinfo.Version,
		GoVersion: runtime.Version(),
		Commit:    buildinfo.Commit,
		Dirty:     buildinfo.DirtyBool(),
	}
}
