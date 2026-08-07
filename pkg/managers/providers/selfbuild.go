package providers

import (
	"context"
	"fmt"
)

// SelfBuild is a synthetic, non-fetching provider: its id ("self-build")
// names the always-offered move/normalize destination for entries authored
// directly in the repo (pkg/tui/actions_normalize.go), so they get their own
// declared icon in the TUI instead of falling back to unknownProviderIcon.
// It never claims an address — self-build entries are created by hand or via
// `skm create`, never acquired through `skm import`.
type SelfBuild struct{}

// NewSelfBuild returns the built-in SelfBuild provider.
func NewSelfBuild() *SelfBuild { return &SelfBuild{} }

// ID returns the provider id.
func (SelfBuild) ID() string { return "self-build" }

// Label returns the human label.
func (SelfBuild) Label() string { return "Self-built" }

// Capability describes what SelfBuild handles: nothing fetchable.
func (SelfBuild) Capability() Capability {
	return Capability{
		ID: "self-build", Label: "Self-built",
		Description: "Entries authored directly in the repo, never imported from an external source",
		Icon:        "🍺",
	}
}

// Normalize returns address unchanged; self-build entries have no address.
func (SelfBuild) Normalize(address string) (string, error) { return address, nil }

// CanHandle always reports false: self-build entries are never acquired
// through import, so this provider must never be auto-matched.
func (SelfBuild) CanHandle(address string) bool { return false }

// Fetch always fails: SelfBuild has nothing to fetch (CanHandle is always
// false, so this should never be called).
func (SelfBuild) Fetch(_ context.Context, address string) (string, error) {
	return "", fmt.Errorf("self-build: nothing to fetch for %q", address)
}
