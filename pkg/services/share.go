package services

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/providers"
)

// ShareEntry carries only the source identity of one selected local entry —
// never its file contents. A recipient always re-fetches from Source.
type ShareEntry struct {
	Name   string           `json:"name"`
	Kind   common.EntryKind `json:"kind"`
	Source string           `json:"source"`
}

// SharePayload is the decoded envelope.
type SharePayload struct {
	Entries []ShareEntry
}

// ShareEntrySummary is the display-oriented view of one entry in a create
// result.
type ShareEntrySummary struct {
	Name   string           `json:"name"`
	Kind   common.EntryKind `json:"kind"`
	Source string           `json:"source"`
}

// ShareCreateResult is the CLI/TUI-facing result of `share create`.
type ShareCreateResult struct {
	Payload  string              `json:"payload"`
	Command  string              `json:"command"`
	Entries  []ShareEntrySummary `json:"entries"`
	Warnings []string            `json:"warnings,omitempty"`
}

// ShareItemResult's outcome is independent per entry: one entry's failure
// never overwrites another's result.
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
// compatible target still counts as success.
type ShareApplyResult struct {
	Items   []ShareItemResult `json:"items"`
	Success bool              `json:"success"`
}

// ShareProgressFunc reports collection progress for a multi-entry
// CreateShare selection; done is 1-indexed. It is called synchronously from
// the collection loop, so it must return quickly.
type ShareProgressFunc func(done, total int, name string)

// CreateShare never embeds an entry's file contents: every shared entry
// carries only a source address a recipient re-fetches from. An entry with no
// address of its own — no upstream origin, and (for a local/self-build entry)
// no shareable repository location — cannot be shared at all; there is no
// content fallback.
func (s *Services) CreateShare(_ context.Context, names []string, onProgress ShareProgressFunc) (*ShareCreateResult, error) {
	entries, err := s.shareEntries(names)
	if err != nil {
		return nil, err
	}
	explicit := len(names) > 0
	total := len(entries)
	report := func(i int, name string) {
		if onProgress != nil {
			onProgress(i+1, total, name)
		}
	}

	var warnings []string
	items := make([]ShareEntry, 0, len(entries))
	summaries := make([]ShareEntrySummary, 0, len(entries))
	seen := make(map[string]bool, len(entries))

	for i, entry := range entries {
		report(i, entry.Name)
		key := string(entry.Kind) + "\x00" + entry.Name
		if seen[key] {
			continue
		}
		source, ok := s.shareSource(entry)
		if !ok {
			reason := fmt.Sprintf("entry %q has no shareable source", entry.Name)
			if explicit {
				return nil, common.WithExitCode(fmt.Errorf("share: %s", reason), common.ExitObject)
			}
			warnings = append(warnings, reason)
			continue
		}
		seen[key] = true
		items = append(items, ShareEntry{Name: entry.Name, Kind: entry.Kind, Source: source})
		summaries = append(summaries, ShareEntrySummary{Name: entry.Name, Kind: entry.Kind, Source: source})
	}
	if len(items) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("share: no eligible entries have a shareable source"), common.ExitObject)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return entryLess(summaries[i].Kind, summaries[i].Name, summaries[j].Kind, summaries[j].Name)
	})

	token, err := encodeSharePayload(SharePayload{Entries: items})
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	return &ShareCreateResult{
		Payload:  token,
		Command:  "skm share apply '" + token + "'",
		Entries:  summaries,
		Warnings: warnings,
	}, nil
}

// shareSource resolves the address a recipient re-fetches entry from: its own
// upstream origin when it has a valid one, or — for a directory-style entry
// with none (a local/self-build skill or command) — this repository's own git
// remote pointed at the entry's subdirectory, the same address shape
// deploy/export already use for the whole repository (repoOrigin,
// providers.BrowseTreeURL). A single-file command has no subdirectory of its
// own to address without also pulling in sibling files, so it is not offered
// this fallback.
func (s *Services) shareSource(entry *common.Entry) (string, bool) {
	if entry.Origin != nil && shareableAddress(entry.Origin.Address) {
		return entry.Origin.Address, true
	}
	if !entry.IsDirectory() {
		return "", false
	}
	remote, branch := s.repoOrigin(), s.repoBranch()
	if remote == "" || branch == "" {
		return "", false
	}
	rel := filepath.ToSlash(s.Repo.RelPath(entry.Path))
	return providers.BrowseTreeURL(remote, branch, rel)
}

// shareEntries resolves the selection: no names selects every active entry;
// an explicit name that does not resolve to an active entry is a hard error,
// independent of shareability.
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

// ApplyShare validates the whole payload before touching the repository, then
// processes each entry independently: a same-named active entry is skipped
// without overwrite, and one entry's failure never erases another's success.
func (s *Services) ApplyShare(ctx context.Context, token string) (*ShareApplyResult, error) {
	payload, err := decodeSharePayload(token)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	result := &ShareApplyResult{Items: []ShareItemResult{}, Success: true}
	for _, entry := range payload.Entries {
		item := s.applyShareEntry(ctx, entry)
		if item.Status == "skipped" || item.Status == "failed" {
			result.Success = false
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *Services) applyShareEntry(ctx context.Context, entry ShareEntry) ShareItemResult {
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
// import, not a failure.
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
