package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// updateEntry is the shared per-entry update runner behind both updateSelected
// and batchUpdate: refresh one entry from its origin and report updated vs
// current. ref is the entry's path, so same-named entries in different
// providers resolve uniquely (FindEntry prefers path matches). Errors carry the
// entry name so the failure surfaces who failed, not just the reason (FR-005).
func (m *model) updateEntry(name, ref string) func(ctx context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		result, err := m.svc.Update(ctx, ref, services.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		verb := "updated"
		if !result.Changed {
			verb = "current"
		}
		return fmt.Sprintf("%s is %s", name, verb), nil
	}
}

// updateSelected refreshes the current entry from its origin in the
// background (key "p").
func (m *model) updateSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if !m.svc.Updatable(entry) {
		reason := "has no origin"
		if entry.Status != common.StatusActive {
			reason = fmt.Sprintf("is %s (only active entries are updatable)", entry.Status)
		}
		m.setStatus(fmt.Sprintf("%s %s; nothing to update", entry.Name, reason))
		return
	}
	m.submitJob("update "+entry.Name, m.updateEntry(entry.Name, entry.Path))
}

// batchUpdateCandidates returns the updatable entries in the current provider
// tab (not the search-narrowed list): batch update acts on the whole tab, so a
// leftover search can't silently shrink what gets refreshed. Which entries are
// updatable is the services layer's rule (svc.Updatable); the TUI only applies
// its own tab scope on top.
func (m *model) batchUpdateCandidates() []*common.Entry {
	tab := m.activeProviderTab()
	var updatable []*common.Entry
	for _, e := range m.entries {
		if matchesProviderTab(e, tab) && m.svc.Updatable(e) {
			updatable = append(updatable, e)
		}
	}
	return updatable
}

// batchUpdate confirms, then refreshes every active-with-origin entry in the
// current tab as one background job per entry (key "P"), so each update is
// observable in the status bar and the task center. Confirming first means a
// tab-wide refresh can't run by accident.
func (m *model) batchUpdate() {
	cands := m.batchUpdateCandidates()
	if len(cands) == 0 {
		m.setStatus("batch update: nothing to update in this tab")
		return
	}
	noun := "entries"
	if len(cands) == 1 {
		noun = "entry"
	}
	m.confirm = &pages.Confirm{
		Prompt: fmt.Sprintf("Update %d %s in this tab from their origins?", len(cands), noun),
		OnYes: func() {
			for _, e := range cands {
				m.submitJob("update "+e.Name, m.updateEntry(e.Name, e.Path))
			}
		},
	}
}
