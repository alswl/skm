package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/tui/components"
)

// pickerItem is one selectable row in a picker modal.
type pickerItem struct {
	label   string
	value   string
	checked bool
}

// picker is a reusable modal: a multi-select checkbox list (targets, discovered
// skills) or a single-select radio list (provider, kind). Selection is confirmed
// with Enter and cancelled with Esc; onDelete, when set, wires a secondary
// destructive action (discover delete) to the Delete key. (FR-036/037/038)
type picker struct {
	title     string
	hint      string
	items     []pickerItem
	cursor    int
	single    bool
	onConfirm func(sel []pickerItem)
	onDelete  func(sel []pickerItem)
}

func (p *picker) move(delta int) {
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor > len(p.items)-1 {
		p.cursor = len(p.items) - 1
	}
}

func (p *picker) toggle() {
	if p.single || len(p.items) == 0 {
		return
	}
	p.items[p.cursor].checked = !p.items[p.cursor].checked
}

// selection returns the confirmed items: the highlighted one for a radio
// picker, otherwise every checked item.
func (p *picker) selection() []pickerItem {
	if p.single {
		if len(p.items) == 0 {
			return nil
		}
		return []pickerItem{p.items[p.cursor]}
	}
	var out []pickerItem
	for _, it := range p.items {
		if it.checked {
			out = append(out, it)
		}
	}
	return out
}

// confirm is a yes/no modal guarding a destructive action (FR-040).
type confirm struct {
	prompt string
	onYes  func()
}

// handlePickerKey drives the active picker modal.
func (m *model) handlePickerKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picker
	switch {
	case key.Matches(msg, m.keys.MoveUp):
		p.move(-1)
	case key.Matches(msg, m.keys.MoveDown):
		p.move(1)
	case key.Matches(msg, m.keys.Toggle):
		p.toggle()
	case key.Matches(msg, m.keys.Enter):
		sel := p.selection()
		onConfirm := p.onConfirm
		m.picker = nil
		if onConfirm != nil {
			onConfirm(sel)
		}
	case key.Matches(msg, m.keys.Delete):
		if p.onDelete != nil {
			sel := p.selection()
			onDelete := p.onDelete
			m.picker = nil
			onDelete(sel)
		}
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Quit):
		m.picker = nil
	}
	return nil
}

// handleConfirmKey drives the active confirmation modal.
func (m *model) handleConfirmKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Yes), key.Matches(msg, m.keys.Enter):
		onYes := m.confirm.onYes
		m.confirm = nil
		if onYes != nil {
			onYes()
		}
	case key.Matches(msg, m.keys.No), key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Quit):
		m.confirm = nil
	}
	return nil
}

// pickerView renders the active picker as a full-screen framed page.
func (m model) pickerView() string {
	p := m.picker
	inner := maxInt(20, m.width) - 2
	var body strings.Builder
	for i, it := range p.items {
		mark := "[ ]"
		switch {
		case p.single && it.checked:
			mark = "(•)"
		case p.single:
			mark = "( )"
		case it.checked:
			mark = "[x]"
		}
		row := mark + " " + it.label
		if i == p.cursor {
			body.WriteString(components.FitCell("  ▶ "+row, inner, components.StyleCursor) + "\n")
		} else {
			body.WriteString(components.FitCell("    "+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	return m.framedPage(" skm · "+p.title+" ", strings.TrimRight(body.String(), "\n"), p.hint)
}

// confirmView renders the active confirmation as a full-screen framed page.
func (m model) confirmView() string {
	body := components.StylePrompt.Render(m.confirm.prompt)
	return m.framedPage(" skm · confirm ", body, "[y] yes   [n/esc/q] no")
}

// framedPage renders title/body/hint inside the box-drawing frame, matching the
// list and detail pages so modals read as full-screen nnn-style views.
func (m model) framedPage(title, body, hint string) string {
	w := maxInt(20, m.width)
	h := maxInt(10, m.height)
	inner := w - 2
	rows := maxInt(1, h-4) // top, [body], sep, hint, bottom
	var sb strings.Builder
	sb.WriteString(components.FrameTop(inner, title) + "\n")
	for _, l := range components.PadLines(components.SplitLines(body), inner, rows) {
		sb.WriteString("│" + components.FitCell(l, inner, lipgloss.NewStyle()) + "│\n")
	}
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(hint, inner, components.StyleStatusBar) + "│\n")
	sb.WriteString(components.FrameBottom(inner))
	return sb.String()
}
