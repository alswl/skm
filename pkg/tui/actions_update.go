package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
)

// updateSelected refreshes the current entry from its origin in the
// background (key "p").
func (m *model) updateSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusActive || entry.Origin == nil {
		m.status = fmt.Sprintf("%s has no origin; nothing to update", entry.Name)
		return
	}
	name := entry.Name
	m.submitJob("update "+name, func(ctx context.Context) (any, error) {
		result, err := m.svc.Update(ctx, name, services.UpdateOptions{})
		if err != nil {
			return nil, err
		}
		verb := "updated"
		if !result.Changed {
			verb = "current"
		}
		return fmt.Sprintf("%s is %s", name, verb), nil
	})
}

// batchUpdate refreshes all active entries with an origin in the background
// (key "P"). The result names which entries failed and why, not just an
// aggregate count (FR-005) — previously the reason was discarded before it
// ever reached the TUI (services.BatchUpdateResult.Failed used to be
// []string).
func (m *model) batchUpdate() {
	m.submitJob("batch-update", func(ctx context.Context) (any, error) {
		res := m.svc.BatchUpdate(ctx, false)
		msg := fmt.Sprintf("batch-update: updated=%d current=%d failed=%d skipped=%d",
			len(res.Updated), len(res.Current), len(res.Failed), len(res.Skipped))
		if len(res.Failed) > 0 {
			reasons := make([]string, len(res.Failed))
			for i, f := range res.Failed {
				reasons[i] = fmt.Sprintf("%s (%s)", f.Name, f.Reason)
			}
			msg += "; failed: " + strings.Join(reasons, ", ")
		}
		return msg, nil
	})
}
