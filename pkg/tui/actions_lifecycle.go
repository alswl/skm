package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
)

// archiveSelected archives or unarchives the current entry (key "a"). Archiving
// is destructive-ish (it uninstalls first, FR-013) so it runs behind a
// confirmation; unarchiving is a plain restore. Both run on the queue (FR-042).
func (m *model) archiveSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	name := entry.Name
	if entry.Status == common.StatusArchived {
		m.submitJob("unarchive "+name, func(ctx context.Context) (any, error) {
			if _, err := m.svc.Unarchive(ctx, name, services.LifecycleOptions{}); err != nil {
				return nil, err
			}
			return "unarchived " + name, nil
		})
		return
	}
	m.confirm = &confirm{
		prompt: fmt.Sprintf("Archive %q? It will be uninstalled from all targets first.", name),
		onYes: func() {
			m.submitJob("archive "+name, func(ctx context.Context) (any, error) {
				// Uninstall first (managed installs only), then archive (FR-013).
				if _, err := m.svc.Uninstall(ctx, name, services.InstallOptions{}); err != nil {
					return nil, err
				}
				if _, err := m.svc.Archive(ctx, name, services.LifecycleOptions{}); err != nil {
					return nil, err
				}
				return "archived " + name, nil
			})
		},
	}
}

// deleteSelected permanently deletes the current entry after confirmation (key
// "d", FR-040). The TUI never deletes a user's real installed file; delete only
// removes a repository entry. The delete runs on the queue (FR-042).
func (m *model) deleteSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	name := m.filtered[m.cursor].Name
	m.confirm = &confirm{
		prompt: fmt.Sprintf("Delete %q from the repository permanently?", name),
		onYes: func() {
			m.submitJob("delete "+name, func(ctx context.Context) (any, error) {
				if _, err := m.svc.Delete(ctx, name, services.LifecycleOptions{Force: true}); err != nil {
					return nil, err
				}
				return "deleted " + name, nil
			})
		},
	}
}
