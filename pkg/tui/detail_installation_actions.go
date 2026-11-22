package tui

import (
	"fmt"
	"strconv"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

func installationLocation(item common.InstallationObservation) string {
	if item.Directory == "" {
		return "User / " + item.Target
	}
	return item.Directory + " / " + item.Target
}

func (m *model) openDetailInstallations() {
	if m.cursor >= len(m.filtered) || m.filtered[m.cursor].Kind != common.KindSkill {
		return
	}
	if m.detailMachineLoading {
		m.setStatus("Scanning installations…")
		return
	}
	if m.detailMachineError != nil {
		m.setStatus(m.detailMachineError.Error())
		return
	}
	entry := m.filtered[m.cursor]
	ref := m.svc.Repo.RelPath(entry.Path)
	positions := append([]common.InstallationObservation(nil), m.detailMachineItems...)
	seenUser := make(map[string]bool)
	for _, item := range positions {
		if item.Directory == "" {
			seenUser[item.Target] = true
		}
	}
	for _, cell := range m.installs.forEntry(entry.Path) {
		if !seenUser[cell.name] && (cell.state == common.InstallConflict || cell.state == common.InstallDangling) {
			positions = append(positions, common.InstallationObservation{Target: cell.name, Status: string(cell.state)})
		}
	}
	items := make([]pages.PickerItem, 0, len(positions))
	for i, item := range positions {
		if item.Target != "" {
			items = append(items, pages.PickerItem{Label: installationLocation(item) + " · " + item.Status, Value: strconv.Itoa(i)})
		}
	}
	if len(items) == 0 {
		m.setStatus("No installation locations found")
		return
	}
	seq := m.detailMachineSequence
	m.picker = &pages.Picker{Title: "installation locations · " + entry.Name, Single: true, Hint: "enter actions  esc back", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || !m.showDetail || m.detailMachineSequence != seq {
			return
		}
		index, err := strconv.Atoi(selected[0].Value)
		if err == nil && index >= 0 && index < len(positions) {
			m.openInstallationLocationActions(entry, ref, positions[index], seq)
			if m.location != nil {
				m.location.back = func() {
					m.openDetailInstallations()
					if m.picker != nil && index < len(m.picker.Items) {
						m.picker.Cursor = index
					}
				}
			}
		}
	}}
}

func (m *model) openInstallationLocationActions(entry *common.Entry, ref string, item common.InstallationObservation, seq uint64) {
	if !m.showDetail || m.detailMachineSequence != seq {
		return
	}
	location := &installationLocationState{
		item: item,
		refresh: func() tea.Cmd {
			m.startMachineSkillScan()
			cmd := m.machinePending
			m.machinePending = nil
			return cmd
		},
	}
	state := m.installState(entry.Path, item.Target)
	if entry.Status == common.StatusActive && item.Directory == "" && (state == common.InstallConflict || state == common.InstallDangling) {
		location.repair = func() { m.prepareSelectedFix(entry, ref, []string{item.Target}) }
	}
	m.openLocation(location)
}

func unlinkPrompt(plan services.MachineUnlinkPlan) string {
	return fmt.Sprintf("Unlink %s from %s at %s?", plan.Skill, plan.Target, plan.Path)
}
