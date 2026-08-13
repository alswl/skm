package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// normalizeSelected starts relocating the currently viewed detail-page
// entry, if it's non-standard or active (key "n" from the detail page): choose
// a provider, preview the destination, then confirm before moving anything.
// Moving an active entry relinks its installs to the new location.
func (m *model) normalizeSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusNonStandard && entry.Status != common.StatusActive {
		m.setStatus(fmt.Sprintf("%s is %s; only non-standard or active entries can be moved", entry.Name, entry.Status))
		return
	}
	m.openNormalizeProviderPicker(entry.Name, m.svc.Repo.RelPath(entry.Path))
}

// openNormalizeProviderPicker lists "local" (the safe default) and
// "self-build" (the user's own skills — always offered) plus every other
// provider already present in the repository, so a non-standard entry can be
// moved to whichever one actually fits it — "移动到更合理的 providers" —
// rather than always defaulting to "local".
func (m *model) openNormalizeProviderPicker(name, ref string) {
	items := []pages.PickerItem{
		{Label: "local", Value: "local"},
		{Label: "self-build", Value: "self-build"},
	}
	seen := map[string]bool{"local": true, "self-build": true}
	for _, t := range m.providerTabs {
		if t == tabAll || t == tabNone || seen[t] {
			continue
		}
		seen[t] = true
		items = append(items, pages.PickerItem{Label: t, Value: t})
	}
	m.picker = &pages.Picker{
		Title:  "move " + name + " → provider",
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Single: true,
		Items:  items,
		OnConfirm: func(sel []pages.PickerItem) {
			provider := "local"
			if len(sel) > 0 {
				provider = sel[0].Value
			}
			m.confirmNormalize(name, ref, provider)
		},
	}
}

// confirmNormalize previews the destination for provider and, once the user
// confirms, runs the move as a background job.
func (m *model) confirmNormalize(name, ref, provider string) {
	preview, err := m.svc.Normalize(m.ctx, ref, provider, services.LifecycleOptions{DryRun: true})
	if err != nil {
		m.setStatus("normalize: " + err.Error())
		return
	}
	m.confirm = &pages.Confirm{
		Prompt: fmt.Sprintf("Move %q to %s?", name, preview.Path),
		OnYes: func() {
			m.submitJob("normalize "+name, func(ctx context.Context) (any, error) {
				res, err := m.svc.Normalize(ctx, ref, provider, services.LifecycleOptions{})
				if err != nil {
					return nil, err
				}
				return fmt.Sprintf("moved %s to %s", name, res.Path), nil
			})
		},
	}
}
