package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/managers"
)

// normalizeSelected starts relocating the currently viewed detail-page
// entry, if it's non-standard (key "m" from the detail page): choose a
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
	items := []pickerItem{
		{label: "local", value: "local"},
		{label: "self-build", value: "self-build"},
	}
	seen := map[string]bool{"local": true, "self-build": true}
	for _, t := range m.providerTabs {
		if t == tabAll || t == tabNone || seen[t] {
			continue
		}
		seen[t] = true
		items = append(items, pickerItem{label: t, value: t})
	}
	m.picker = &picker{
		title:  "move " + name + " → provider",
		hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		single: true,
		items:  items,
		onConfirm: func(sel []pickerItem) {
			provider := "local"
			if len(sel) > 0 {
				provider = sel[0].value
			}
			m.confirmNormalize(name, provider)
		},
	}
}

// confirmNormalize previews the destination for provider and, once the user
// confirms, runs the move as a background job.
func (m *model) confirmNormalize(name, provider string) {
	preview, err := m.svc.Normalize(m.ctx, name, provider, managers.LifecycleOptions{DryRun: true})
	if err != nil {
		m.status = "normalize: " + err.Error()
		return
	}
	m.confirm = &confirm{
		prompt: fmt.Sprintf("Move %q to %s?", name, preview.Path),
		onYes: func() {
			m.submitJob("normalize "+name, func(ctx context.Context) (any, error) {
				res, err := m.svc.Normalize(ctx, name, provider, managers.LifecycleOptions{})
				if err != nil {
					return nil, err
				}
				return fmt.Sprintf("moved %s to %s", name, res.Path), nil
			})
		},
	}
}
