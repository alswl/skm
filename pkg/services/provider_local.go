package services

import (
	"context"

	"github.com/alswl/skm/skm/pkg/dal"
)

// Local handles real local paths/directories as acquisition sources. In auto
// mode the import manager prefers a real local source directly (FR-020); the
// provider still participates in registration order for completeness.
type Local struct{}

// NewLocal returns the built-in Local provider.
func NewLocal() *Local { return &Local{} }

// ID returns the provider id.
func (Local) ID() string { return "local" }

// Label returns the human label.
func (Local) Label() string { return "Local filesystem" }

// Capability describes what Local handles.
func (Local) Capability() Capability {
	return Capability{
		ID: "local", Label: "Local filesystem",
		Description: "Imports from an existing local path or directory",
		Schemes:     []string{"/", "./", "~/"},
		Icon:        "📂",
	}
}

// Normalize expands paths entered outside a shell and trims pasted whitespace.
func (Local) Normalize(address string) (string, error) { return normalizeImportSource(address), nil }

// CanHandle reports whether address is an existing local path.
func (Local) CanHandle(address string) bool {
	return dal.PathExists(normalizeImportSource(address))
}

// Fetch returns the local source path itself; the import manager copies it
// into the repository under a transaction.
func (Local) Fetch(_ context.Context, address string) (string, error) {
	return normalizeImportSource(address), nil
}

// borrowsSource reports whether p.Fetch hands back a path the caller already
// owns instead of a fresh temp staging directory. Only Local does, and what it
// returns is the user's own source: removing it as fetch cleanup would delete
// their files. The id is a safe discriminator — the built-in is registered
// first and Registry.Register rejects later duplicates of an id.
func borrowsSource(p Provider) bool { return p.ID() == (Local{}).ID() }
