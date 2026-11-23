package tui

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	tea "github.com/charmbracelet/bubbletea"
)

type machineSkillScanMsg struct {
	sequence uint64
	ref      string
	items    []common.InstallationObservation
	err      error
}

// openDetail prepares and shows the selected entry's full-screen detail page.
func (m *model) openDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.refreshDetail()
	m.showDetail = true
	m.startMachineSkillScan()
}

func (m *model) startMachineSkillScan() {
	m.detailMachineSequence++
	m.detailOpenLocationsPending = false
	m.detailMachineItems = nil
	m.detailMachineError = nil
	if m.cursor >= len(m.filtered) || m.filtered[m.cursor].Kind != common.KindSkill {
		m.detailMachineLoading = false
		return
	}
	m.detailMachineLoading = true
	ref := m.svc.Repo.RelPath(m.filtered[m.cursor].Path)
	seq, svc := m.detailMachineSequence, m.svc
	m.machinePending = func() tea.Msg {
		all, err := svc.ScanMachineAll()
		var items []common.InstallationObservation
		if err == nil {
			items, err = svc.FilterMachineSkill(all, ref)
		}
		return machineSkillScanMsg{sequence: seq, ref: ref, items: items, err: err}
	}
}

func (m *model) acceptMachineSkillScan(msg machineSkillScanMsg) {
	if !m.showDetail || msg.sequence != m.detailMachineSequence || m.cursor >= len(m.filtered) || m.svc.Repo.RelPath(m.filtered[m.cursor].Path) != msg.ref {
		return
	}
	m.detailMachineLoading = false
	m.detailMachineItems, m.detailMachineError = msg.items, msg.err
	if m.detailOpenLocationsPending {
		m.detailOpenLocationsPending = false
		m.openDetailInstallations()
	}
}

func (m model) detailContent() string {
	if m.cursor >= len(m.filtered) || m.filtered[m.cursor].Kind != common.KindSkill {
		return m.detail
	}
	var out strings.Builder
	out.WriteString(m.detail)
	out.WriteString("\n\nInstallations\n")
	switch {
	case m.detailMachineLoading:
		out.WriteString("  Scanning…\n")
	case m.detailMachineError != nil:
		fmt.Fprintf(&out, "  Error: %s\n", m.detailMachineError)
	case len(m.detailMachineItems) == 0:
		out.WriteString("  None found\n")
	default:
		for _, item := range m.detailMachineItems {
			scope := item.Directory
			if scope == "" {
				scope = "user"
			}
			fmt.Fprintf(&out, "  %s  %s  %s  %s", scope, item.Target, item.Status, item.Path)
			if item.Error != "" {
				fmt.Fprintf(&out, ": %s", item.Error)
			}
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// refreshDetail performs the filesystem-backed detail construction during
// Update, keeping View pure and the availability data cached.
func (m *model) refreshDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.detail = m.buildDetail()
	e := m.filtered[m.cursor]
	m.detailTargets = len(m.installs.forEntry(e.Path))
	m.detailInstalled = m.installs.installedAnywhere(e.Path)
	m.detailOffset = 0
}
