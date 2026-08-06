package common

import (
	"path/filepath"
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

// Origin records the provenance of a remote-fetched entry, stored in
// <entry>/meta.json. It is present only for remote imports/updates (I3).
type Origin struct {
	Address string  `json:"address"`
	ModeID  *string `json:"mode_id,omitempty"`
}

// InstallTarget is a destination directory for installs, from targets.json or
// built-in defaults.
type InstallTarget struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Builtin bool      `json:"builtin"`
	Kind    EntryKind `json:"kind"`
}

// Entry is the central asset — a managed skill or command.
type Entry struct {
	Name        string
	Description string
	Kind        EntryKind
	Status      Status
	Path        string
	Version     *string
	ModeID      *string
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

// ModeIDValue returns the mode_id, or "" when unset.
func (e *Entry) ModeIDValue() string {
	if e.ModeID == nil {
		return ""
	}
	return *e.ModeID
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

// MatchTarget reports whether a target receives this kind of entry.
func (e *Entry) MatchTarget(t InstallTarget) bool { return t.Kind == e.Kind }

// DescriptionLower returns the lowercased description for search matching.
func (e *Entry) DescriptionLower() string { return strings.ToLower(e.Description) }

// NameLower returns the lowercased name for search matching.
func (e *Entry) NameLower() string { return strings.ToLower(e.Name) }
