package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/managers"
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
	items := []pickerItem{{label: "auto (local first, else match by order)", value: ""}}
	for _, p := range m.svc.Registry.Providers() {
		items = append(items, pickerItem{label: p.ID() + " — " + p.Label(), value: p.ID()})
	}
	m.picker = &picker{
		title:  "import provider",
		hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		items:  items,
		single: true,
		onConfirm: func(sel []pickerItem) {
			provider := ""
			if len(sel) > 0 {
				provider = sel[0].value
			}
			m.openKindPicker(addr, provider)
		},
	}
}

// openKindPicker asks which kind the import should be treated as: auto-detect,
// or an explicit skill/command hint (FR-037).
func (m *model) openKindPicker(addr, provider string) {
	m.picker = &picker{
		title:  "import kind",
		hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		single: true,
		items: []pickerItem{
			{label: "auto (detect from marker)", value: "auto"},
			{label: "skill", value: "skill"},
			{label: "command", value: "command"},
		},
		onConfirm: func(sel []pickerItem) {
			kind := "auto"
			if len(sel) > 0 {
				kind = sel[0].value
			}
			m.runImport(addr, provider, kind)
		},
	}
}

// runImport imports an address through the shared services layer in the
// background, honoring the chosen provider and kind (FR-037).
func (m *model) runImport(addr, provider, kind string) {
	m.submitJob("import "+addr, func(ctx context.Context) (any, error) {
		result, err := m.svc.Import(ctx, addr, managers.ImportOptions{Provider: provider, Kind: kind})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("imported %s (%s) via %s", result.Name, result.Type, result.Provider), nil
	})
}
