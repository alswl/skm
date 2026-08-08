package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/pages"
)

// normalizeSelected starts relocating the currently viewed detail-page
// entry, if it's non-standard (key "n" from the detail page): choose a
// provider, preview the destination, then confirm before moving anything.
func (m *model) normalizeSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusNonStandard {
		m.status = fmt.Sprintf("%s is %s; only non-standard entries can be moved", entry.Name, entry.Status)
		return
	}
	m.openNormalizeProviderPicker(entry.Name)
}

// openNormalizeProviderPicker lists "local" (the safe default) and
// "self-build" (the user's own skills — always offered) plus every other
// provider already present in the repository, so a non-standard entry can be
// moved to whichever one actually fits it — "移动到更合理的 providers" —
// rather than always defaulting to "local".
func (m *model) openNormalizeProviderPicker(name string) {
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
			m.confirmNormalize(name, provider)
		},
	}
}

// confirmNormalize previews the destination for provider and, once the user
// confirms, runs the move as a background job.
func (m *model) confirmNormalize(name, provider string) {
	preview, err := m.svc.Normalize(m.ctx, name, provider, services.LifecycleOptions{DryRun: true})
	if err != nil {
		m.status = "normalize: " + err.Error()
		return
	}
	m.confirm = &pages.Confirm{
		Prompt: fmt.Sprintf("Move %q to %s?", name, preview.Path),
		OnYes: func() {
			m.submitJob("normalize "+name, func(ctx context.Context) (any, error) {
				res, err := m.svc.Normalize(ctx, name, provider, services.LifecycleOptions{})
				if err != nil {
					return nil, err
				}
				return fmt.Sprintf("moved %s to %s", name, res.Path), nil
			})
		},
	}
}
