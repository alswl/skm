// Package version holds build-time version metadata injected via -ldflags
// (makefile-go build target): -X github.com/alswl/skm/skm/pkg/version.Version=…
// Values stay "dev"/"none"/"unknown" for plain `go build` (no ldflags).
// Intentionally test-free (003-engineering-optimization FR-014): pure data,
// no logic.
package version

import "runtime/debug"

// Build metadata, populated by the Makefile (alswl/makefile-go) at link time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// BuildDate returns the explicitly injected build date when available. Local
// builds use Go's embedded VCS timestamp as a reliable fallback instead of
// exposing the unhelpful "built unknown" placeholder.
func BuildDate() string {
	if Date != "unknown" {
		return Date
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Date
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.time" && setting.Value != "" {
			return setting.Value
		}
	}
	return Date
}
