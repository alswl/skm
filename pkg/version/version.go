// Package version holds build-time version metadata injected via -ldflags
// (makefile-go build target): -X github.com/alswl/skm/skm/pkg/version.Version=…
// Values stay "dev"/"none"/"unknown" for plain `go build` (no ldflags).
package version

// Build metadata, populated by the Makefile (alswl/makefile-go) at link time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
