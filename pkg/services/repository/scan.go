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
// A marker found anywhere else under the repo root is also surfaced, as a
// StatusNonStandard entry, instead of being silently invisible.
func (r *Repository) Scan() []*common.Entry {
	var entries []*common.Entry
	entries = append(entries, r.scanTop("skills", common.KindSkill, common.StatusActive)...)
	entries = append(entries, r.scanTop("commands", common.KindCommand, common.StatusActive)...)
	entries = append(entries, r.scanTop("archived", "", common.StatusArchived)...)
	entries = append(entries, r.scanNonStandard()...)
	return entries
}

// managedTopDirs are the only top-level names scanNonStandard does not
// itself walk into — they already have their own dedicated scan above.
var managedTopDirs = map[string]bool{"skills": true, "commands": true, "archived": true}

// scanNonStandard walks the repo root looking for skill/command markers
// outside skills/commands/archived, so a misplaced asset is visible instead
// of invisible. Only directory markers (a dir containing SKILL.md or
// command.md) are detected — loose *.md files are not, since any arbitrary
// markdown file (README, docs) would otherwise be a false positive.
func (r *Repository) scanNonStandard() []*common.Entry {
	var out []*common.Entry
	children, err := os.ReadDir(r.root)
	if err != nil {
		return out
	}
	for _, c := range children {
		if !c.IsDir() || managedTopDirs[c.Name()] || strings.HasPrefix(c.Name(), ".") {
			continue
		}
		out = append(out, r.walkNonStandard(filepath.Join(r.root, c.Name()))...)
	}
	return out
}

// walkNonStandard recurses into dir until it finds a marker (stopping there,
// like scanGroup does for the managed trees) or runs out of subdirectories.
func (r *Repository) walkNonStandard(dir string) []*common.Entry {
	switch {
	case dal.PathExists(filepath.Join(dir, "SKILL.md")):
		return []*common.Entry{r.buildNonStandardEntry(dir, common.KindSkill)}
	case dal.PathExists(filepath.Join(dir, "command.md")):
		return []*common.Entry{r.buildNonStandardEntry(dir, common.KindCommand)}
	}
	var out []*common.Entry
	children, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, c := range children {
		if !c.IsDir() || strings.HasPrefix(c.Name(), ".") {
			continue
		}
		out = append(out, r.walkNonStandard(filepath.Join(dir, c.Name()))...)
	}
	return out
}

// buildNonStandardEntry builds a StatusNonStandard entry for a marker found
// outside the managed trees. Frontmatter is parsed best-effort — a
// misplaced-but-otherwise-valid marker still shows its real name/description;
// a broken one falls back to the directory's basename — but the entry is
// always flagged non-standard, since location, not frontmatter validity, is
// the problem being reported.
func (r *Repository) buildNonStandardEntry(dir string, kind common.EntryKind) *common.Entry {
	e := &common.Entry{Status: common.StatusNonStandard, Path: dir, Kind: kind, Name: filepath.Base(dir)}
	rel, err := filepath.Rel(r.root, dir)
	if err != nil {
		rel = dir
	}
	note := fmt.Sprintf("non-standard location %q; expected under %s/<provider>/[<group>/]<name>/", rel, kind.TopDir())
	e.Error = strPtr(note)

	data, err := os.ReadFile(filepath.Join(dir, kind.MarkerFile()))
	if err != nil {
		return e
	}
	fm, _, err := dal.ParseFrontmatter(data)
	if err != nil {
		return e
	}
	if fm.Name != "" {
		e.Name = fm.Name
	}
	e.Description = fm.Description
	if fm.Version != nil && *fm.Version != "" {
		e.Version = fm.Version
	}
	return e
}

// buildNonStandardFileEntry builds a non-standard entry for a single-file
// command found directly under commands/ with no provider directory
// (commands/<name>.md instead of the required commands/<provider>/<name>.md).
func (r *Repository) buildNonStandardFileEntry(path string) *common.Entry {
	e := &common.Entry{Status: common.StatusNonStandard, Path: path, Kind: common.KindCommand, Name: fileStem(path)}
	rel, err := filepath.Rel(r.root, path)
	if err != nil {
		rel = path
	}
	e.Error = strPtr(fmt.Sprintf("non-standard location %q; expected under commands/<provider>/<name>.md", rel))

	data, err := os.ReadFile(path)
	if err != nil {
		return e
	}
	fm, _, err := dal.ParseFrontmatter(data)
	if err != nil {
		return e
	}
	if fm.Name != "" {
		e.Name = fm.Name
	}
	e.Description = fm.Description
	if fm.Version != nil && *fm.Version != "" {
		e.Version = fm.Version
	}
	return e
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
		p := filepath.Join(top, d.Name())
		if !d.IsDir() {
			// A loose command file directly under commands/, with no
			// provider directory, is missing the required nesting level
			// (commands/<provider>/<name>.md) — flag it instead of silently
			// skipping it. Skills have no single-file form, so a loose file
			// directly under skills/ is left alone (more likely a stray
			// README than a misplaced skill).
			if kind == common.KindCommand && isMarkdown(d.Name()) {
				out = append(out, r.buildNonStandardFileEntry(p))
			}
			continue
		}
		if actualKind, ok := markerKind(p); ok {
			// The would-be "provider" directory directly holds the marker:
			// the provider level itself is missing
			// (skills/<name>/SKILL.md instead of
			// skills/<provider>/<name>/SKILL.md) — flag it rather than
			// treating d.Name() as a provider and never finding the entry.
			out = append(out, r.buildNonStandardEntry(p, actualKind))
			continue
		}
		out = append(out, r.scanProvider(p, d.Name(), kind, status)...)
	}
	return out
}

// markerKind reports which marker dir holds (preferring SKILL.md), used to
// resolve the kind of an entry found at a non-standard nesting depth where
// the expected kind may be ambiguous (e.g. archived/).
func markerKind(dir string) (common.EntryKind, bool) {
	if dal.PathExists(filepath.Join(dir, "SKILL.md")) {
		return common.KindSkill, true
	}
	if dal.PathExists(filepath.Join(dir, "command.md")) {
		return common.KindCommand, true
	}
	return "", false
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
	e := &common.Entry{Status: status, Path: dir, ProviderID: strPtr(modeID)}
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
	e := &common.Entry{Status: status, Path: path, Kind: common.KindCommand, ProviderID: strPtr(modeID)}
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
			if o.ProviderID != nil && *o.ProviderID != "" && *o.ProviderID != *e.ProviderID {
				return errEntry(e, fmt.Sprintf("mode_id mismatch: origin %q != directory %q", *o.ProviderID, *e.ProviderID))
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
