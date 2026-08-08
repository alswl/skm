package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/tui/pages"
)

// handlePickerKey drives the active picker modal.
func (m *model) handlePickerKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picker
	switch {
	case key.Matches(msg, m.keys.MoveUp):
		p.MoveCursor(-1)
	case key.Matches(msg, m.keys.MoveDown):
		p.MoveCursor(1)
	case key.Matches(msg, m.keys.Toggle):
		p.ToggleCurrent()
	case key.Matches(msg, m.keys.Enter):
		sel := p.Selection()
		onConfirm := p.OnConfirm
		m.picker = nil
		if onConfirm != nil {
			onConfirm(sel)
		}
	case key.Matches(msg, m.keys.Delete):
		if p.OnDelete != nil {
			sel := p.Selection()
			onDelete := p.OnDelete
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
		onYes := m.confirm.OnYes
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
	return pages.PickerView(m.width, m.height, m.status, m.picker)
}

// confirmView renders the active confirmation as a full-screen framed page.
func (m model) confirmView() string {
	return pages.ConfirmView(m.width, m.height, m.status, m.confirm)
}

// framedPage forwards to pages.FramedPage, keeping the remaining root-side
// callers (tasks/targets views, until they move to pages themselves) working
// unchanged.
func (m model) framedPage(title, body, hint string) string {
	return pages.FramedPage(m.width, m.height, title, body, m.status, hint)
}
