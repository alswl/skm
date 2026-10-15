package tui

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/components"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

func newImportInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Cursor.Style = components.StyleInputCursor
	input.Cursor.SetMode(cursor.CursorStatic)
	input.Focus()
	return input
}

func (m model) importInputWidth() int {
	inner := maxInt(20, m.width) - 2
	// textinput renders one extra cell for its cursor. Reserve that cell so
	// the cursor remains inside the frame when the viewport is full.
	return maxInt(1, inner-lipgloss.Width("Import address: ")-1)
}

func (m *model) resizeImportInput() {
	m.importInput.Width = m.importInputWidth()
	m.importInput.SetCursor(m.importInput.Position())
}

func (m *model) openImportPrompt() {
	m.importing = true
	m.importAddr = ""
	m.importCursor = 0
	m.importInput = newImportInput()
	m.resizeImportInput()
}

// Local paths need spaces and in-place corrections, not just URL-like input.
func (m *model) handleImportKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.importing = false
		addr := m.importInput.Value()
		m.importAddr = ""
		m.importCursor = 0
		m.importInput.Reset()
		if addr == "" {
			return nil
		}
		m.openProviderPicker(addr)
		return nil
	case key.Matches(msg, m.keys.Esc):
		m.importing = false
		m.importAddr = ""
		m.importCursor = 0
		m.importInput.Reset()
		return nil
	default:
		var cmd tea.Cmd
		m.resizeImportInput()
		if msg.Type == tea.KeySpace {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
		}
		m.importInput, cmd = m.importInput.Update(msg)
		m.importAddr = m.importInput.Value()
		m.importCursor = m.importInput.Position()
		return cmd
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
			return selectionJobResult{
				status: fmt.Sprintf("imported %s (%s) via %s", result.Name, result.Type, result.Provider),
				path:   m.svc.Repo.RelPath(result.Path),
				reveal: true,
			}, nil
		}
	}
	m.submitJobForce("import "+addr, attempt(false), attempt(true))
}
