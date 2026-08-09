package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// fixableTargets returns entry's per-target conflicts (a non-managed object
// already occupies the target) and dangling installs (a stray/broken link
// occupies it) — the two states fixSelected repairs. Both need the same
// repair: installSkill/installClaudeMarkdown/installAdapter (Force) already
// treat InstallConflict and InstallDangling identically (backup-remove
// whatever's there, then relink) — see pkg/services/skill_install.go. A plain
// Uninstall is *not* the fix for dangling: it only ever removes a link that
// currently resolves to entry's exact path (isManagedLink), so it silently
// no-ops on the common case — a stale link left behind after the entry moved,
// now resolving elsewhere.
//
// Read from the model's install state, never probed: this is called from two
// key handlers (F, and x to decide whether to offer the item at all), both on
// the event loop.
func (m *model) fixableTargets(entry *common.Entry) []string {
	return m.installs.broken(entry.Name)
}

// fixSelected repairs the current entry's conflict and dangling installs (key
// "F") by force-installing over each broken target — the same recovery
// already offered for a needs-force failure (see submitJobForce), just
// applied proactively instead of waiting for a rejected install. Scoped to
// the entry under the cursor only.
func (m *model) fixSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusActive {
		m.setStatus(fmt.Sprintf("%s is %s; nothing to fix", entry.Name, entry.Status))
		return
	}
	broken := m.fixableTargets(entry)
	if len(broken) == 0 {
		m.setStatus(fmt.Sprintf("fix: %s has no conflicts or dangling installs", entry.Name))
		return
	}
	name := entry.Name
	prompt := fmt.Sprintf("Fix %s? Overwrite: %s", name, strings.Join(broken, ", "))
	m.confirm = &pages.Confirm{Prompt: prompt, OnYes: func() {
		m.submitJob("fix "+name, func(ctx context.Context) (any, error) {
			result, err := m.svc.Install(ctx, name, services.InstallOptions{Targets: broken, Force: true})
			if err != nil {
				return nil, err
			}
			return installStatusMessage("fix", name, result), nil
		})
	}}
}
