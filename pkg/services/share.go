package services

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

// ShareMode distinguishes an embedded-content payload from a source-only
// upstream payload. A payload has exactly one mode, and it is never inferred
// from which optional fields are present (FR-007).
type ShareMode string

const (
	ShareModeContent  ShareMode = "content"
	ShareModeUpstream ShareMode = "upstream"
)

// ContentFile is one file inside a ContentEntry, path relative to the entry
// root (data-model.md ContentEntry).
type ContentFile struct {
	Path  string `json:"path"`
	Mode  uint32 `json:"mode"`
	Bytes []byte `json:"-"`
}

// ContentEntry carries the complete contents of one selected local entry.
type ContentEntry struct {
	Name  string           `json:"name"`
	Kind  common.EntryKind `json:"kind"`
	Files []ContentFile    `json:"files"`
}

// UpstreamEntry carries only the source identity of one selected entry.
type UpstreamEntry struct {
	Name   string           `json:"name"`
	Kind   common.EntryKind `json:"kind"`
	Source string           `json:"source"`
}

// SharePayload is the decoded envelope: exactly one of Content or Upstream is
// populated, matching Mode (data-model.md SharePayload).
type SharePayload struct {
	Mode     ShareMode
	Content  []ContentEntry
	Upstream []UpstreamEntry
}

// ShareEntrySummary is the display-oriented view of one entry in a create
// result; Source is only set for upstream payloads.
type ShareEntrySummary struct {
	Name   string           `json:"name"`
	Kind   common.EntryKind `json:"kind"`
	Source string           `json:"source,omitempty"`
}

type ShareCreateResult struct {
	Payload  string              `json:"payload"`
	Command  string              `json:"command"`
	Mode     ShareMode           `json:"mode"`
	Entries  []ShareEntrySummary `json:"entries"`
	Warnings []string            `json:"warnings,omitempty"`
}

// ShareItemResult's outcome is independent per entry: one entry's failure
// never overwrites another's result (data-model.md).
type ShareItemResult struct {
	Name    string                 `json:"name"`
	Kind    common.EntryKind       `json:"kind"`
	Source  string                 `json:"source,omitempty"`
	Status  string                 `json:"status"`
	Reason  string                 `json:"reason,omitempty"`
	Results []common.InstallReport `json:"results,omitempty"`
}

// ShareApplyResult is the aggregate result of `share apply`. Success is false
// only when at least one item was skipped or failed; an item imported with no
// compatible target still counts as success (FR-013/SC-007).
type ShareApplyResult struct {
	Items   []ShareItemResult `json:"items"`
	Success bool              `json:"success"`
}

// CreateShare never falls back to local content for an entry lacking a valid
// upstream source in upstream mode (FR-009).
func (s *Services) CreateShare(_ context.Context, names []string, upstream bool) (*ShareCreateResult, error) {
	entries, err := s.shareEntries(names)
	if err != nil {
		return nil, err
	}
	explicit := len(names) > 0

	payload := SharePayload{}
	var warnings []string
	summaries := make([]ShareEntrySummary, 0, len(entries))
	seen := make(map[string]bool, len(entries))

	if upstream {
		payload.Mode = ShareModeUpstream
		for _, entry := range entries {
			key := string(entry.Kind) + "\x00" + entry.Name
			if seen[key] {
				continue
			}
			if entry.Origin == nil || !shareableAddress(entry.Origin.Address) {
				reason := fmt.Sprintf("entry %q has no valid upstream source", entry.Name)
				if explicit {
					return nil, common.WithExitCode(fmt.Errorf("share: %s", reason), common.ExitObject)
				}
				warnings = append(warnings, reason)
				continue
			}
			seen[key] = true
			payload.Upstream = append(payload.Upstream, UpstreamEntry{Name: entry.Name, Kind: entry.Kind, Source: entry.Origin.Address})
			summaries = append(summaries, ShareEntrySummary{Name: entry.Name, Kind: entry.Kind, Source: entry.Origin.Address})
		}
		if len(payload.Upstream) == 0 {
			return nil, common.WithExitCode(fmt.Errorf("share: no eligible entries have a valid upstream source"), common.ExitObject)
		}
	} else {
		payload.Mode = ShareModeContent
		for _, entry := range entries {
			key := string(entry.Kind) + "\x00" + entry.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			content, entryWarnings, err := collectContentEntry(entry)
			if err != nil {
				return nil, common.WithExitCode(fmt.Errorf("share: %w", err), common.ExitError)
			}
			warnings = append(warnings, entryWarnings...)
			payload.Content = append(payload.Content, content)
			summaries = append(summaries, ShareEntrySummary{Name: entry.Name, Kind: entry.Kind})
		}
		if len(payload.Content) == 0 {
			return nil, common.WithExitCode(fmt.Errorf("share: no eligible entries to share"), common.ExitObject)
		}
	}

	sort.Slice(summaries, func(i, j int) bool {
		return entryLess(summaries[i].Kind, summaries[i].Name, summaries[j].Kind, summaries[j].Name)
	})

	token, err := encodeSharePayload(payload)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	return &ShareCreateResult{
		Payload:  token,
		Command:  "skm share apply '" + token + "'",
		Mode:     payload.Mode,
		Entries:  summaries,
		Warnings: warnings,
	}, nil
}

// shareEntries resolves the selection: no names selects every active entry;
// an explicit name that does not resolve to an active entry is a hard error
// (FR-004 edge case), independent of upstream eligibility.
func (s *Services) shareEntries(names []string) ([]*common.Entry, error) {
	if len(names) == 0 {
		var out []*common.Entry
		for _, entry := range s.Scan() {
			if entry.Status == common.StatusActive {
				out = append(out, entry)
			}
		}
		return out, nil
	}
	out := make([]*common.Entry, 0, len(names))
	for _, name := range names {
		entry, err := s.ResolveEntry(name)
		if err != nil {
			return nil, err
		}
		if entry == nil || entry.Status != common.StatusActive {
			return nil, common.WithExitCode(fmt.Errorf("share: entry %q not found", name), common.ExitObject)
		}
		out = append(out, entry)
	}
	return out, nil
}

func shareableAddress(address string) bool {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "ssh"
}

// collectContentEntry warns and skips a symlink rather than failing the whole
// selection (FR-005/FR-006).
func collectContentEntry(entry *common.Entry) (ContentEntry, []string, error) {
	kind := entry.Kind
	var files []ContentFile
	var warnings []string

	if !entry.IsDirectory() {
		data, err := os.ReadFile(entry.Path)
		if err != nil {
			return ContentEntry{}, nil, fmt.Errorf("read %q: %w", entry.Path, err)
		}
		if len(data) > maxShareContentFileSize {
			return ContentEntry{}, nil, fmt.Errorf("entry %q exceeds the file size limit", entry.Name)
		}
		files = append(files, ContentFile{Path: kind.MarkerFile(), Mode: 0o644, Bytes: data})
		return ContentEntry{Name: entry.Name, Kind: kind, Files: files}, warnings, nil
	}

	root := entry.Path
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			warnings = append(warnings, fmt.Sprintf("skipped symlink %q in %q", relSlash, entry.Name))
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if len(data) > maxShareContentFileSize {
			return fmt.Errorf("entry %q file %q exceeds the file size limit", entry.Name, relSlash)
		}
		files = append(files, ContentFile{Path: relSlash, Mode: uint32(info.Mode().Perm()), Bytes: data})
		return nil
	})
	if walkErr != nil {
		return ContentEntry{}, nil, walkErr
	}
	if len(files) > maxShareFilesPerEntry {
		return ContentEntry{}, nil, fmt.Errorf("entry %q has too many files", entry.Name)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return ContentEntry{Name: entry.Name, Kind: kind, Files: files}, warnings, nil
}

// ApplyShare validates the whole payload before touching the repository, then
// processes each entry independently: a same-named active entry is skipped
// without overwrite, and one entry's failure never erases another's success
// (FR-010, FR-014, FR-016).
func (s *Services) ApplyShare(ctx context.Context, token string) (*ShareApplyResult, error) {
	payload, err := decodeSharePayload(token)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	result := &ShareApplyResult{Items: []ShareItemResult{}, Success: true}
	switch payload.Mode {
	case ShareModeContent:
		for _, entry := range payload.Content {
			item := s.applyContentEntry(ctx, entry)
			if item.Status == "skipped" || item.Status == "failed" {
				result.Success = false
			}
			result.Items = append(result.Items, item)
		}
	case ShareModeUpstream:
		for _, entry := range payload.Upstream {
			item := s.applyUpstreamEntry(ctx, entry)
			if item.Status == "skipped" || item.Status == "failed" {
				result.Success = false
			}
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

// shareContentProvider is the meta.json provider bucket for imported content
// payloads: they have no remote origin, so they are bucketed like a
// self-authored entry rather than attributed to a fabricated source.
const shareContentProvider = "shared"

func (s *Services) applyContentEntry(ctx context.Context, entry ContentEntry) ShareItemResult {
	item := ShareItemResult{Name: entry.Name, Kind: entry.Kind}
	if existing := s.findActiveByIdentity(entry.Name, entry.Kind); existing != nil {
		item.Status, item.Reason = "skipped", fmt.Sprintf("an entry named %q already exists", entry.Name)
		return item
	}
	staged, err := stageContentEntry(entry)
	if err != nil {
		item.Status, item.Reason = "failed", err.Error()
		return item
	}
	defer func() { _ = os.RemoveAll(staged) }()

	imported, err := s.Repo.ImportStaged(ctx, staged, shareContentProvider, "", false, nil)
	if err != nil {
		item.Status, item.Reason = "failed", err.Error()
		return item
	}
	return s.finishShareInstall(ctx, item, imported.Path)
}

func (s *Services) applyUpstreamEntry(ctx context.Context, entry UpstreamEntry) ShareItemResult {
	item := ShareItemResult{Name: entry.Name, Kind: entry.Kind, Source: entry.Source}
	if existing := s.findActiveByIdentity(entry.Name, entry.Kind); existing != nil {
		item.Status, item.Reason = "skipped", fmt.Sprintf("an entry named %q already exists", entry.Name)
		return item
	}
	imported, err := s.Import(ctx, entry.Source, ImportOptions{Kind: string(entry.Kind)})
	if err != nil {
		item.Status, item.Reason = "failed", err.Error()
		return item
	}
	return s.finishShareInstall(ctx, item, imported.Path)
}

// finishShareInstall treats an empty compatible-target set as a preserved
// import, not a failure (FR-013).
func (s *Services) finishShareInstall(ctx context.Context, item ShareItemResult, entryPath string) ShareItemResult {
	entry, err := s.ResolveEntry(s.Repo.RelPath(entryPath))
	if err != nil || entry == nil {
		item.Status, item.Reason = "failed", "imported entry could not be resolved"
		return item
	}
	installed, err := s.Install(ctx, s.Repo.RelPath(entry.Path), InstallOptions{})
	if err != nil {
		item.Status, item.Reason = "failed", err.Error()
		return item
	}
	item.Results = installed.Results
	if len(installed.Results) == 0 {
		item.Status, item.Reason = "imported", "no compatible installer target was found"
		return item
	}
	anyChanged := false
	for _, report := range installed.Results {
		if report.Changed {
			anyChanged = true
		}
	}
	if anyChanged {
		item.Status = "installed"
	} else {
		item.Status = "already_present"
	}
	return item
}

func (s *Services) findActiveByIdentity(name string, kind common.EntryKind) *common.Entry {
	for _, entry := range s.Scan() {
		if entry.Status == common.StatusActive && entry.Kind == kind && entry.Name == name {
			return entry
		}
	}
	return nil
}

// stageContentEntry writes a ContentEntry's files into a fresh temp directory
// so it can be validated and placed through the existing repository import
// path (research.md #3). Paths were already validated at decode time; the
// join is still defensive against an unexpected empty component.
func stageContentEntry(entry ContentEntry) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-share-*")
	if err != nil {
		return "", err
	}
	for _, f := range entry.Files {
		if !validShareContentPath(f.Path) {
			_ = os.RemoveAll(tmp)
			return "", fmt.Errorf("entry %q has an unsafe file path %q", entry.Name, f.Path)
		}
		dest := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
		mode := os.FileMode(f.Mode).Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dest, f.Bytes, mode); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
	}
	return tmp, nil
}
