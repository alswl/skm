package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/pages"
)

// handleImportKey processes keys while the import dialog is active: printable
// characters append to the address, Enter runs the import, Esc cancels
// (tui-contract.md key "i").
func (m *model) handleImportKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.importing = false
		addr := m.importAddr
		m.importAddr = ""
		if addr == "" {
			return nil
		}
		m.openProviderPicker(addr)
		return nil
	case key.Matches(msg, m.keys.Esc):
		m.importing = false
		m.importAddr = ""
		return nil
	case key.Matches(msg, m.keys.Backspace):
		if len(m.importAddr) > 0 {
			m.importAddr = m.importAddr[:len(m.importAddr)-1]
		}
		return nil
	default:
		if msg.Type == tea.KeyRunes {
			m.importAddr += string(msg.Runes)
		}
		return nil
	}
}

// openProviderPicker asks which provider to use for the import: "auto" (the
// default local-first / registration-order matching) or a specific registered
// provider by id (FR-037).
func (m *model) openProviderPicker(addr string) {
	items := []pages.PickerItem{{Label: "auto (local first, else match by order)", Value: ""}}
	for _, p := range m.svc.Registry.Providers() {
		items = append(items, pages.PickerItem{Label: p.ID() + " — " + p.Label(), Value: p.ID()})
	}
	m.picker = &pages.Picker{
		Title:  "import provider",
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Items:  items,
		Single: true,
		OnConfirm: func(sel []pages.PickerItem) {
			provider := ""
			if len(sel) > 0 {
				provider = sel[0].Value
			}
			m.openKindPicker(addr, provider)
		},
	}
}

// openKindPicker asks which kind the import should be treated as: auto-detect,
// or an explicit skill/command hint (FR-037).
func (m *model) openKindPicker(addr, provider string) {
	m.picker = &pages.Picker{
		Title:  "import kind",
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Single: true,
		Items: []pages.PickerItem{
			{Label: "auto (detect from marker)", Value: "auto"},
			{Label: "skill", Value: "skill"},
			{Label: "command", Value: "command"},
		},
		OnConfirm: func(sel []pages.PickerItem) {
			kind := "auto"
			if len(sel) > 0 {
				kind = sel[0].Value
			}
			m.runImport(addr, provider, kind)
		},
	}
}

// runImport imports an address through the shared services layer in the
// background, honoring the chosen provider and kind (FR-037). An import whose
// name collides with an existing entry fails with a needs-force error
// (Repository.ImportStaged); submitJobForce turns that into a confirm-then-
// retry offer instead of a dead end.
func (m *model) runImport(addr, provider, kind string) {
	attempt := func(force bool) func(ctx context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			result, err := m.svc.Import(ctx, addr, services.ImportOptions{Provider: provider, Kind: kind, Force: force})
			if err != nil {
				return nil, err
			}
			return fmt.Sprintf("imported %s (%s) via %s", result.Name, result.Type, result.Provider), nil
		}
	}
	m.submitJobForce("import "+addr, attempt(false), attempt(true))
}
