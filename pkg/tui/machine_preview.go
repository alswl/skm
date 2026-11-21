package tui

import tea "github.com/charmbracelet/bubbletea"

// queueMachinePreview records the request so stale or failed plans can be
// explicitly re-evaluated without re-entering the selection.
func (m *model) queueMachinePreview(run func(uint64) tea.Msg) {
	if !m.machine.open {
		return
	}
	m.machine.retryPreview = func() { m.queueMachinePreview(run) }
	m.machine.sequence++
	m.machine.scanning = true
	m.machine.err = nil
	sequence := m.machine.sequence
	m.machinePending = func() tea.Msg { return run(sequence) }
}
