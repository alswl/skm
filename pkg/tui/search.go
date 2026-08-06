package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleSearchKey processes keys while the search input is active. Printable
// characters append, backspace deletes, Enter confirms, Esc cancels/clears
// (tui-contract.md: "/" enters real-time filtering on name + description).
func (m *model) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.searching = false
	case key.Matches(msg, m.keys.Esc):
		m.search = ""
		m.searching = false
		m.refreshFiltered()
	case key.Matches(msg, m.keys.Backspace):
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.refreshFiltered()
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.search += string(msg.Runes)
			m.refreshFiltered()
		}
	}
	return nil
}
