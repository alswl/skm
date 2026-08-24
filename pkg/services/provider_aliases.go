package services

import "github.com/alswl/skm/skm/pkg/providers"

// Provider domain types live in pkg/providers; aliases keep the services
// facade source-compatible for CLI/TUI integrations during the split.
type Provider = providers.Provider
type Capability = providers.Capability
type ProviderError = providers.ProviderError
type ProviderLoadFailure = providers.ProviderLoadFailure
type BuiltinProviderDefinition = providers.BuiltinProviderDefinition

const (
	CodeUnsupportedAddress = providers.CodeUnsupportedAddress
	CodeNormalizeFailed    = providers.CodeNormalizeFailed
	CodeFetchFailed        = providers.CodeFetchFailed
	CodeProtocolError      = providers.CodeProtocolError
	CodeTimeout            = providers.CodeTimeout
	CodeDuplicateID        = providers.CodeDuplicateID
	CodeEmptyID            = providers.CodeEmptyID
)

func NewLocal() *providers.Local         { return providers.NewLocal() }
func NewSelfBuild() *providers.SelfBuild { return providers.NewSelfBuild() }
func NewGitHub() Provider                { return providers.NewGitHub() }
func NewGitLab() Provider                { return providers.NewGitLab() }
func NewSkillsSh() Provider              { return providers.NewSkillsSh() }

func BuiltinProviderDefinitions() []BuiltinProviderDefinition {
	return providers.BuiltinProviderDefinitions()
}

func BuiltinProviders() ([]Provider, error) { return providers.BuiltinProviders() }

func borrowsSource(p Provider) bool { return p.ID() == "local" }
