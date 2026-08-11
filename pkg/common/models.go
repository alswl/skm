package common

import (
	"path/filepath"
	"slices"
	"strings"
)

// EntryKind is the kind of a managed asset: skill or command. It determines
// the top-level tree and the marker file (data-model.md).
type EntryKind string

const (
	KindSkill   EntryKind = "skill"
	KindCommand EntryKind = "command"
)

// MarkerFile returns the marker filename for a skill/command entry. A
// single-file command uses its own name as the marker instead.
func (k EntryKind) MarkerFile() string {
	switch k {
	case KindSkill:
		return "SKILL.md"
	case KindCommand:
		return "command.md"
	}
	return ""
}

// TopDir returns the repository subdirectory for the kind.
func (k EntryKind) TopDir() string {
	switch k {
	case KindSkill:
		return "skills"
	case KindCommand:
		return "commands"
	}
	return ""
}

// Status is the lifecycle status of an entry, derived from tree location and
// validity (data-model.md).
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
	StatusError    Status = "error"
	// StatusNonStandard marks a skill/command marker found outside the
	// managed skills/commands/archived trees (e.g. loose in the repo root).
	// It is scanned and reported so it's visible, but never installable —
	// only StatusActive entries can be installed/updated.
	StatusNonStandard Status = "non_standard"
)

// InstallState is the derived health of an entry within a target. It is never
// persisted (FR-019).
type InstallState string

const (
	InstallAbsent    InstallState = "absent"
	InstallInstalled InstallState = "installed"
	InstallConflict  InstallState = "conflict"
	InstallDangling  InstallState = "dangling"
)

// Origin records the provenance of an imported entry, stored in
// <entry>/meta.json. Address is the source URL or local path, ProviderID the
// provider id (persisted as "mode_id" for backward compatibility — 003 A-1),
// and Path the entry's location relative to the repository root — together
// they let an installed skill's meta.json track its source and layout.
type Origin struct {
	Address    string  `json:"address"`
	ProviderID *string `json:"mode_id,omitempty"`
	Path       string  `json:"path,omitempty"`
}

// InstallStrategy is the on-disk shape a target uses to place an asset
// (data-model.md). These are the three shapes the installer already produces;
// naming them lets a target *declare* which one it uses instead of the
// installer hardcoding it by tool name (002-open-provider-target FR-013).
type InstallStrategy string

const (
	StrategySkillSymlink   InstallStrategy = "skill-symlink"
	StrategyCommandMarker  InstallStrategy = "command-marker"
	StrategyCommandAdapter InstallStrategy = "command-adapter"

	// pluginStrategyPrefix marks a strategy as delegated to an out-of-process
	// Target plugin instead of one of the three built-in shapes above (see
	// IsPlugin/PluginID). The id after the prefix is the plugin's own stable
	// id, matched against discovered plugins at install/validate time.
	pluginStrategyPrefix = "plugin:"
)

// PluginStrategy returns the "plugin:<id>" strategy value referring to the
// Target plugin with the given id.
func PluginStrategy(id string) InstallStrategy {
	return InstallStrategy(pluginStrategyPrefix + id)
}

// IsPlugin reports whether the strategy delegates to a Target plugin rather
// than a built-in shape.
func (s InstallStrategy) IsPlugin() bool {
	return strings.HasPrefix(string(s), pluginStrategyPrefix)
}

// PluginID returns the plugin id a "plugin:<id>" strategy refers to, or ""
// when IsPlugin is false.
func (s InstallStrategy) PluginID() string {
	if !s.IsPlugin() {
		return ""
	}
	return strings.TrimPrefix(string(s), pluginStrategyPrefix)
}

// CompatibleWith reports whether a strategy is a valid on-disk shape for kind
// (target-config.md: skill→skill-symlink; command→command-marker|command-adapter).
// A plugin-delegated strategy is accepted structurally for any kind here;
// whether the referenced plugin actually supports that kind is a runtime
// check against its declared Capability, not a schema-level one (mirrors a
// Provider's CanHandle being dynamic rather than declared in targets.json).
func (s InstallStrategy) CompatibleWith(kind EntryKind) bool {
	if s.IsPlugin() {
		return true
	}
	switch kind {
	case KindSkill:
		return s == StrategySkillSymlink
	case KindCommand:
		return s == StrategyCommandMarker || s == StrategyCommandAdapter
	}
	return false
}

// InstallTarget is a destination directory for installs, from targets.json or
// built-in defaults. Accepts/Strategies declare which kinds it receives and
// how (002-open-provider-target FR-012); Kind is the legacy single-kind field,
// kept for v1 targets.json backward compatibility during migration.
type InstallTarget struct {
	Name       string                        `json:"name"`
	Platform   string                        `json:"platform,omitempty"`
	Path       string                        `json:"path"`
	Builtin    bool                          `json:"builtin"`
	Kind       EntryKind                     `json:"kind,omitempty"`
	Accepts    []EntryKind                   `json:"accepts,omitempty"`
	Strategies map[EntryKind]InstallStrategy `json:"strategies,omitempty"`
}

// AcceptsKind reports whether the target receives installs of kind, per
// EffectiveAccepts.
func (t InstallTarget) AcceptsKind(kind EntryKind) bool {
	return slices.Contains(t.EffectiveAccepts(), kind)
}

// EffectiveAccepts returns Accepts when set (v2), or the accepts derived from
// the legacy Kind field (data-model.md Migration Mapping, research R6) when
// not. This is the single source of truth for v1→v2 kind semantics: a
// Kind:"skill" target accepts both skill (its own kind) and command (via a
// command-adapter); a Kind:"command" target accepts only command.
func (t InstallTarget) EffectiveAccepts() []EntryKind {
	if len(t.Accepts) > 0 {
		return t.Accepts
	}
	accepts, _ := legacyKindDefaults(t.Kind)
	return accepts
}

// EffectiveStrategy returns the strategy Strategies[kind] declares when set
// (v2), or the strategy the legacy Kind field implies for kind (research R6)
// when not. ok is false when kind isn't accepted at all.
func (t InstallTarget) EffectiveStrategy(kind EntryKind) (strategy InstallStrategy, ok bool) {
	if len(t.Strategies) > 0 {
		strategy, ok = t.Strategies[kind]
		return strategy, ok
	}
	_, strategies := legacyKindDefaults(t.Kind)
	strategy, ok = strategies[kind]
	return strategy, ok
}

// legacyKindDefaults maps a v1 targets.json {kind} value to its v2
// accepts/strategies, reproducing 001's installer.go dispatch exactly
// (data-model.md Migration Mapping): a skill-kind target also receives
// commands via a command-adapter; a command-kind target receives only
// commands, as a command-marker.
func legacyKindDefaults(kind EntryKind) ([]EntryKind, map[EntryKind]InstallStrategy) {
	switch kind {
	case KindSkill:
		return []EntryKind{KindSkill, KindCommand}, map[EntryKind]InstallStrategy{
			KindSkill:   StrategySkillSymlink,
			KindCommand: StrategyCommandAdapter,
		}
	case KindCommand:
		return []EntryKind{KindCommand}, map[EntryKind]InstallStrategy{
			KindCommand: StrategyCommandMarker,
		}
	}
	return nil, nil
}

// Entry is the central asset — a managed skill or command.
type Entry struct {
	Name        string
	Description string
	Kind        EntryKind
	Status      Status
	Path        string
	Version     *string
	ProviderID  *string
	Group       *string
	Error       *string
	Origin      *Origin
}

// InstallReport is the per-target result of an install/uninstall action.
type InstallReport struct {
	Target  string       `json:"target"`
	Status  InstallState `json:"status"`
	Changed bool         `json:"changed"`
}

// MarkerPath returns the absolute path of the entry's marker file. For
// single-file commands the marker is the file itself (FR-005).
func (e *Entry) MarkerPath() string {
	if e.Kind == KindCommand && strings.HasSuffix(e.Path, ".md") {
		return e.Path
	}
	return filepath.Join(e.Path, e.Kind.MarkerFile())
}

// ProviderIDValue returns the provider id (persisted as "mode_id"), or "" when unset.
func (e *Entry) ProviderIDValue() string {
	if e.ProviderID == nil {
		return ""
	}
	return *e.ProviderID
}

// GroupValue returns the group, or "" when unset.
func (e *Entry) GroupValue() string {
	if e.Group == nil {
		return ""
	}
	return *e.Group
}

// VersionValue returns the version, or "" when unset.
func (e *Entry) VersionValue() string {
	if e.Version == nil {
		return ""
	}
	return *e.Version
}

// IsActive reports whether the entry is in the active tree.
func (e *Entry) IsActive() bool { return e.Status == StatusActive }

// IsDirectory reports whether the entry is a directory entry (as opposed to a
// single-file command whose path ends in .md). Only directory entries are
// kind-convertible (FR-026).
func (e *Entry) IsDirectory() bool {
	return !strings.HasSuffix(e.Path, ".md")
}

// DescriptionLower returns the lowercased description for search matching.
func (e *Entry) DescriptionLower() string { return strings.ToLower(e.Description) }

// NameLower returns the lowercased name for search matching.
func (e *Entry) NameLower() string { return strings.ToLower(e.Name) }
