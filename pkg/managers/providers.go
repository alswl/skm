package managers

import (
	"github.com/alswl/skm/skm/pkg/services/providers"
)

// ProviderInfo is one row of `provider list`/`validate` (contracts/cli-json.md).
type ProviderInfo struct {
	ID          string                   `json:"id"`
	Label       string                   `json:"label,omitempty"`
	Kind        string                   `json:"kind"` // builtin | plugin
	Description string                   `json:"description,omitempty"`
	Schemes     []string                 `json:"schemes,omitempty"`
	Path        string                   `json:"path,omitempty"`
	Loaded      bool                     `json:"loaded"`
	Error       *providers.ProviderError `json:"error"`
}

// ProviderListResult is the CLI JSON report for `provider list`.
type ProviderListResult struct {
	Providers []ProviderInfo `json:"providers"`
}

// ProviderValidateEntry is one result of `provider validate`.
type ProviderValidateEntry struct {
	ID    string                   `json:"id"`
	OK    bool                     `json:"ok"`
	Error *providers.ProviderError `json:"error"`
}

// ProviderValidateResult is the CLI JSON report for `provider validate`.
type ProviderValidateResult struct {
	Results []ProviderValidateEntry `json:"results"`
	Success bool                    `json:"success"`
}

// ProviderList reports every registered provider in resolution order, plus
// any plugin that failed to load, each with its specific reason (FR-006).
func (s *Services) ProviderList() *ProviderListResult {
	res := &ProviderListResult{Providers: []ProviderInfo{}}
	for _, p := range s.Registry.Providers() {
		cap := p.Capability()
		res.Providers = append(res.Providers, ProviderInfo{
			ID: p.ID(), Label: p.Label(), Kind: providerKind(p),
			Description: cap.Description, Schemes: cap.Schemes, Loaded: true,
		})
	}
	for _, f := range s.Registry.LoadFailures() {
		reason := f.Reason
		res.Providers = append(res.Providers, ProviderInfo{
			ID: f.ID, Kind: "plugin", Path: f.Path, Loaded: false, Error: &reason,
		})
	}
	return res
}

// ProviderValidate reports pass/fail-with-reason for every provider, or just
// id when non-empty (SC-009). A loaded provider is always ok: it already
// proved usable during discovery/registration.
func (s *Services) ProviderValidate(id string) *ProviderValidateResult {
	res := &ProviderValidateResult{Success: true}
	for _, info := range s.ProviderList().Providers {
		if id != "" && info.ID != id {
			continue
		}
		entry := ProviderValidateEntry{ID: info.ID, OK: info.Loaded, Error: info.Error}
		if !entry.OK {
			res.Success = false
		}
		res.Results = append(res.Results, entry)
	}
	return res
}

// ProviderIcons collects each registered provider's declared icon
// (Capability().Icon), keyed by provider id — a presentation view-model so
// the TUI never has to import pkg/managers/providers directly (003
// engineering-optimization F-05).
func (s *Services) ProviderIcons() map[string]string {
	icons := make(map[string]string, len(s.Registry.Providers()))
	for _, p := range s.Registry.Providers() {
		if icon := p.Capability().Icon; icon != "" {
			icons[p.ID()] = icon
		}
	}
	return icons
}

// providerKind reports "plugin" for subprocess providers and "builtin" for
// everything else.
func providerKind(p providers.Provider) string {
	if _, ok := p.(*providers.PluginProvider); ok {
		return "plugin"
	}
	return "builtin"
}
