package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// Repository provides filesystem access to a skill/command repository rooted
// at root (the repo root containing skills/ and commands/).
type Repository struct {
	root string
}

// New returns a Repository for the given root.
func New(root string) *Repository { return &Repository{root: root} }

// Root returns the repository root path.
func (r *Repository) Root() string { return r.root }

// Scan walks skills/, commands/ and archived/ and builds the entry list.
// Invalid assets become visible error entries; the scan never aborts
// (FR-006). Global name conflicts are not flagged here — verify reports them.
func (r *Repository) Scan() []*common.Entry {
	var entries []*common.Entry
	entries = append(entries, r.scanTop("skills", common.KindSkill, common.StatusActive)...)
	entries = append(entries, r.scanTop("commands", common.KindCommand, common.StatusActive)...)
	entries = append(entries, r.scanTop("archived", "", common.StatusArchived)...)
	return entries
}

// scanTop walks a top-level tree (skills/commands/archived).
func (r *Repository) scanTop(topDir string, kind common.EntryKind, status common.Status) []*common.Entry {
	var out []*common.Entry
	top := filepath.Join(r.root, topDir)
	dirs, err := os.ReadDir(top)
	if err != nil {
		return out
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		p := filepath.Join(top, d.Name())
		out = append(out, r.scanProvider(p, d.Name(), kind, status)...)
	}
	return out
}

// scanProvider walks one provider directory; its name is the mode_id.
func (r *Repository) scanProvider(dir, modeID string, kind common.EntryKind, status common.Status) []*common.Entry {
	var out []*common.Entry
	children, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, c := range children {
		p := filepath.Join(dir, c.Name())
		if c.IsDir() {
			if hasMarker(p, kind) {
				out = append(out, r.buildDirEntry(p, modeID, "", kind, status))
			} else {
				out = append(out, r.scanGroup(p, modeID, c.Name(), kind, status)...)
			}
		} else if isMarkdown(c.Name()) && kind == common.KindCommand {
			out = append(out, r.buildFileEntry(p, modeID, "", status))
		}
	}
	return out
}

// scanGroup walks a group directory, recursing into nested groups.
func (r *Repository) scanGroup(dir, modeID, group string, kind common.EntryKind, status common.Status) []*common.Entry {
	var out []*common.Entry
	children, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, c := range children {
		p := filepath.Join(dir, c.Name())
		if c.IsDir() {
			if hasMarker(p, kind) {
				out = append(out, r.buildDirEntry(p, modeID, group, kind, status))
			} else {
				g := group + "/" + c.Name()
				out = append(out, r.scanGroup(p, modeID, g, kind, status)...)
			}
		} else if isMarkdown(c.Name()) && kind == common.KindCommand {
			out = append(out, r.buildFileEntry(p, modeID, group, status))
		}
	}
	return out
}

// hasMarker reports whether a directory holds an entry marker for the kind
// (or either marker for archived entries).
func hasMarker(dir string, kind common.EntryKind) bool {
	switch kind {
	case common.KindSkill:
		return dal.PathExists(filepath.Join(dir, "SKILL.md"))
	case common.KindCommand:
		return dal.PathExists(filepath.Join(dir, "command.md"))
	default:
		return dal.PathExists(filepath.Join(dir, "SKILL.md")) ||
			dal.PathExists(filepath.Join(dir, "command.md"))
	}
}

func isMarkdown(name string) bool { return strings.HasSuffix(name, ".md") }

// buildDirEntry validates a directory entry (skill or directory command).
func (r *Repository) buildDirEntry(dir, modeID, group string, kind common.EntryKind, status common.Status) *common.Entry {
	e := &common.Entry{Status: status, Path: dir, ModeID: strPtr(modeID)}
	if group != "" {
		e.Group = strPtr(group)
	}
	if kind == "" {
		// archived: kind is inferred from the marker file.
		if dal.PathExists(filepath.Join(dir, "SKILL.md")) {
			e.Kind = common.KindSkill
		} else if dal.PathExists(filepath.Join(dir, "command.md")) {
			e.Kind = common.KindCommand
		} else {
			return errEntry(e, "missing marker (SKILL.md or command.md)")
		}
	} else {
		e.Kind = kind
	}
	return r.finishEntry(e, filepath.Join(dir, e.Kind.MarkerFile()), true)
}

// buildFileEntry validates a single-file command (FR-005: name may fall back
// to the file stem).
func (r *Repository) buildFileEntry(path, modeID, group string, status common.Status) *common.Entry {
	e := &common.Entry{Status: status, Path: path, Kind: common.KindCommand, ModeID: strPtr(modeID)}
	if group != "" {
		e.Group = strPtr(group)
	}
	return r.finishEntry(e, path, false)
}

// finishEntry parses the marker frontmatter, enforces required fields, and
// checks origin mode_id consistency (I2). Single-file commands may omit name.
func (r *Repository) finishEntry(e *common.Entry, marker string, requireName bool) *common.Entry {
	data, err := os.ReadFile(marker)
	if err != nil {
		return errEntry(e, "cannot read marker: "+err.Error())
	}
	fm, _, err := dal.ParseFrontmatter(data)
	if err != nil {
		return errEntry(e, err.Error())
	}
	name := fm.Name
	if name == "" {
		if requireName {
			// Keep the entry identifiable by its on-disk identity.
			e.Name = filepath.Base(e.Path)
			return errEntry(e, "frontmatter missing required field: name")
		}
		name = fileStem(marker)
	}
	if fm.Description == "" {
		return errEntry(e, "frontmatter missing required field: description")
	}
	e.Name = name
	e.Description = fm.Description
	if fm.Version != nil && *fm.Version != "" {
		e.Version = fm.Version
	}
	// Origin only exists for directory entries (single-file commands have no
	// meta.json), and its mode_id must match the directory (I2).
	if dal.IsDir(e.Path) {
		if o, err := dal.ReadMeta(e.Path); err == nil {
			e.Origin = o
			if o.ModeID != nil && *o.ModeID != "" && *o.ModeID != *e.ModeID {
				return errEntry(e, fmt.Sprintf("mode_id mismatch: origin %q != directory %q", *o.ModeID, *e.ModeID))
			}
		}
	}
	return e
}

// errEntry marks an entry as an error with the given reason.
func errEntry(e *common.Entry, reason string) *common.Entry {
	e.Status = common.StatusError
	e.Error = strPtr(reason)
	return e
}

func strPtr(s string) *string { return &s }

// fileStem returns the base name without its extension.
func fileStem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
