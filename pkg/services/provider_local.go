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

// Normalize returns address unchanged; local paths have no canonical form
// beyond what the filesystem already resolves.
func (Local) Normalize(address string) (string, error) { return address, nil }

// CanHandle reports whether address is an existing local path.
func (Local) CanHandle(address string) bool {
	return dal.PathExists(address)
}

// Fetch returns the local source path itself; the import manager copies it
// into the repository under a transaction.
func (Local) Fetch(_ context.Context, address string) (string, error) {
	return address, nil
}
