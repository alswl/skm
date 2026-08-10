package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
)

// claimAndRepairSelected adopts a selected skill that the repository scan
// marked invalid or non-standard. It deliberately uses the same Import service
// path as CLI and the normal TUI import flow, so repair, collision handling,
// and transactional relocation have one implementation.
func (m *model) claimAndRepairSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Kind != common.KindSkill || (entry.Status != common.StatusError && entry.Status != common.StatusNonStandard) {
		m.setStatus(fmt.Sprintf("%s cannot be claimed", entry.Name))
		return
	}
	name, path := entry.Name, entry.Path
	m.submitJob("claim "+name, func(ctx context.Context) (any, error) {
		res, err := m.svc.ClaimSkill(ctx, path)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("claimed %s → %s", res.Name, res.Path), nil
	})
}
