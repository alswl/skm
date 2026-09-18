package services

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

// ShareItem is the decoded, portable identity of one entry. Targets are
// deliberately absent: installation expands to all compatible local targets.
type ShareItem struct {
	Source string           `json:"source"`
	Name   string           `json:"name"`
	Kind   common.EntryKind `json:"kind"`
}

type ShareCreateResult struct {
	Payload  string      `json:"payload"`
	Command  string      `json:"command"`
	Items    []ShareItem `json:"items"`
	Warnings []string    `json:"warnings,omitempty"`
}

type SharePreview struct {
	Items []ShareItem `json:"items"`
}

type ShareItemResult struct {
	Source  string                 `json:"source"`
	Name    string                 `json:"name"`
	Kind    common.EntryKind       `json:"kind"`
	Status  string                 `json:"status"`
	Reason  string                 `json:"reason,omitempty"`
	Results []common.InstallReport `json:"results,omitempty"`
}

type ShareInstallResult struct {
	Items   []ShareItemResult `json:"items"`
	Success bool              `json:"success"`
}

// CreateShare emits a portable command; entries without a canonical source
// cannot be represented in a receiver-independent PAYLOAD.
func (s *Services) CreateShare(_ context.Context, names []string) (*ShareCreateResult, error) {
	entries, err := s.shareEntries(names)
	if err != nil {
		return nil, err
	}
	items := make([]ShareItem, 0, len(entries))
	warnings := make([]string, 0)
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Origin == nil || !shareableAddress(entry.Origin.Address) {
			warnings = append(warnings, fmt.Sprintf("skipped %q: no downloadable source URL", entry.Name))
			continue
		}
		key := entry.Origin.Address + "\x00" + string(entry.Kind) + "\x00" + entry.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, ShareItem{Source: entry.Origin.Address, Name: entry.Name, Kind: entry.Kind})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
	if len(items) == 0 {
		if len(warnings) > 0 {
			return nil, common.WithExitCode(fmt.Errorf("share: no installed entries have a downloadable source URL"), common.ExitObject)
		}
		return nil, common.WithExitCode(fmt.Errorf("share: no installed entries to share"), common.ExitObject)
	}
	payload, err := encodeSharePayload(items)
	if err != nil {
		return nil, err
	}
	return &ShareCreateResult{Payload: payload, Command: "skm share install '" + payload + "'", Items: items, Warnings: warnings}, nil
}

func (s *Services) shareEntries(names []string) ([]*common.Entry, error) {
	if len(names) == 0 {
		var out []*common.Entry
		for _, entry := range s.Scan() {
			if entry.Status != common.StatusActive || !s.entryInstalled(entry) {
				continue
			}
			out = append(out, entry)
		}
		return out, nil
	}
	out := make([]*common.Entry, 0, len(names))
	for _, name := range names {
		entry, err := s.ResolveEntry(name)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, common.WithExitCode(fmt.Errorf("share: entry %q not found", name), common.ExitObject)
		}
		if entry.Status != common.StatusActive || !s.entryInstalled(entry) {
			return nil, common.WithExitCode(fmt.Errorf("share: entry %q is not currently installed", name), common.ExitObject)
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *Services) entryInstalled(entry *common.Entry) bool {
	for _, target := range s.Installer.Targets(entry) {
		if s.Installer.State(entry, target) == common.InstallInstalled {
			return true
		}
	}
	return false
}

func shareableAddress(address string) bool {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "ssh"
}

func (s *Services) PreviewShare(payload string) (*SharePreview, error) {
	items, err := decodeSharePayload(payload)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	return &SharePreview{Items: items}, nil
}

// InstallShare retains independent item outcomes so one failed source does
// not cancel the batch.
func (s *Services) InstallShare(ctx context.Context, payload string) (*ShareInstallResult, error) {
	items, err := decodeSharePayload(payload)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	result := &ShareInstallResult{Items: make([]ShareItemResult, 0, len(items)), Success: true}
	for _, item := range items {
		itemResult := ShareItemResult{Source: item.Source, Name: item.Name, Kind: item.Kind}
		entry, err := s.findOrigin(item)
		if err != nil {
			itemResult.Status, itemResult.Reason = "failed", err.Error()
			result.Success = false
			result.Items = append(result.Items, itemResult)
			continue
		}
		if entry == nil {
			imported, importErr := s.Import(ctx, item.Source, ImportOptions{Kind: string(item.Kind)})
			if importErr != nil {
				itemResult.Status, itemResult.Reason = "failed", importErr.Error()
				result.Success = false
				result.Items = append(result.Items, itemResult)
				continue
			}
			entry, err = s.ResolveEntry(s.Repo.RelPath(imported.Path))
			if err != nil || entry == nil {
				itemResult.Status, itemResult.Reason = "failed", "imported entry could not be resolved"
				result.Success = false
				result.Items = append(result.Items, itemResult)
				continue
			}
		}
		installed, installErr := s.Install(ctx, s.Repo.RelPath(entry.Path), InstallOptions{})
		if installErr != nil {
			itemResult.Status, itemResult.Reason = "failed", installErr.Error()
			result.Success = false
			result.Items = append(result.Items, itemResult)
			continue
		}
		itemResult.Results = installed.Results
		itemResult.Status = "installed"
		allUnchanged := len(installed.Results) > 0
		for _, report := range installed.Results {
			if report.Changed {
				allUnchanged = false
			}
		}
		if allUnchanged {
			itemResult.Status = "already_present"
		}
		result.Items = append(result.Items, itemResult)
	}
	return result, nil
}

func (s *Services) findOrigin(item ShareItem) (*common.Entry, error) {
	for _, entry := range s.Scan() {
		if entry.Status != common.StatusActive || entry.Origin == nil {
			continue
		}
		if entry.Origin.Address == item.Source && entry.Kind == item.Kind {
			return entry, nil
		}
	}
	return nil, nil
}
