package services

import "github.com/alswl/skm/skm/pkg/providers"

// Registry is the services facade over the provider domain registry. The
// protocol adapter remains here because it depends on the shared services
// plugin envelope.
type Registry struct{ inner *providers.Registry }

func NewRegistry() *Registry { return &Registry{inner: providers.NewRegistry()} }

func (r *Registry) Register(p Provider) error {
	return r.inner.Register(asProtocolProvider(p))
}

func (r *Registry) Providers() []Provider                          { return r.inner.Providers() }
func (r *Registry) Match(address string) Provider                  { return r.inner.Match(address) }
func (r *Registry) Get(id string) Provider                         { return r.inner.Get(id) }
func (r *Registry) SetLoadFailures(failures []ProviderLoadFailure) { r.inner.SetLoadFailures(failures) }
func (r *Registry) LoadFailures() []ProviderLoadFailure            { return r.inner.LoadFailures() }
