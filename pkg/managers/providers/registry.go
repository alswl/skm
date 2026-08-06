package providers

import (
	"context"
	"fmt"
)

// Provider is an acquisition adapter (data-model.md). Built-ins are Local and
// GitHub; external plugins implement the same interface via a subprocess
// protocol (US8).
type Provider interface {
	// ID is a non-empty unique identifier (duplicates are rejected in favor
	// of the first registered).
	ID() string
	// Label is a non-empty human-readable name.
	Label() string
	// CanHandle reports whether this provider can acquire the address.
	CanHandle(address string) bool
	// Fetch retrieves the asset at address into a fresh temp directory and
	// returns its path. The caller owns cleanup of the returned directory.
	Fetch(ctx context.Context, address string) (string, error)
}

// Registry holds providers in registration order. Auto-import uses the first
// provider that CanHandle an address (FR-020); --provider selects by id.
type Registry struct {
	order []Provider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register appends a provider, rejecting duplicates in favor of the first and
// enforcing non-empty id/label (FR-035).
func (r *Registry) Register(p Provider) error {
	if p.ID() == "" {
		return fmt.Errorf("provider: id must be non-empty")
	}
	if p.Label() == "" {
		return fmt.Errorf("provider %q: label must be non-empty", p.ID())
	}
	for _, existing := range r.order {
		if existing.ID() == p.ID() {
			return fmt.Errorf("provider id %q already registered; duplicate rejected", p.ID())
		}
	}
	r.order = append(r.order, p)
	return nil
}

// Providers returns providers in registration order.
func (r *Registry) Providers() []Provider { return r.order }

// Match returns the first registered provider that can handle address, or nil.
func (r *Registry) Match(address string) Provider {
	for _, p := range r.order {
		if p.CanHandle(address) {
			return p
		}
	}
	return nil
}

// Get returns the provider with the given id, or nil.
func (r *Registry) Get(id string) Provider {
	for _, p := range r.order {
		if p.ID() == id {
			return p
		}
	}
	return nil
}
