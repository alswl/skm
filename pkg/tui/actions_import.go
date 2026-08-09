package tui

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
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
//
// The address the user just typed usually answers this already, so the cursor
// opens on whichever provider would claim it (Registry.Match — the same
// matching "auto" would do) rather than making them walk past the answer.
// Every provider stays listed and the cursor still moves: this pre-selects,
// it does not decide.
func (m *model) openProviderPicker(addr string) {
	detected := ""
	if p := m.svc.Registry.Match(addr); p != nil {
		detected = p.ID()
	}
	items := []pages.PickerItem{{Label: "auto (local first, else match by order)", Value: ""}}
	cursor := 0
	for _, p := range m.svc.Registry.Providers() {
		label := p.ID() + " — " + p.Label()
		if p.ID() == detected {
			label += "  · detected from the address"
			cursor = len(items)
		}
		items = append(items, pages.PickerItem{Label: label, Value: p.ID()})
	}
	m.picker = &pages.Picker{
		Title:  "import provider",
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Items:  items,
		Cursor: cursor,
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

// inferKind reads the entry kind off the address when it names a marker file
// outright — a browse URL ending in SKILL.md is a skill, and nothing else. It
// only pre-positions the picker; "auto" (probe the fetched tree) remains the
// answer whenever the address doesn't say.
func inferKind(addr string) string {
	switch strings.ToLower(path.Base(strings.TrimRight(addr, "/"))) {
	case "skill.md":
		return "skill"
	case "command.md":
		return "command"
	}
	return "auto"
}

// openKindPicker asks which kind the import should be treated as: auto-detect,
// or an explicit skill/command hint (FR-037).
func (m *model) openKindPicker(addr, provider string) {
	items := []pages.PickerItem{
		{Label: "auto (detect from marker)", Value: "auto"},
		{Label: "skill", Value: "skill"},
		{Label: "command", Value: "command"},
	}
	cursor := 0
	if kind := inferKind(addr); kind != "auto" {
		for i, it := range items {
			if it.Value == kind {
				items[i].Label += "  · detected from the address"
				cursor = i
			}
		}
	}
	m.picker = &pages.Picker{
		Title:  "import kind",
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Single: true,
		Items:  items,
		Cursor: cursor,
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
