package tui

import (
	"fmt"
	"strconv"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type detailUnlinkPreviewMsg struct {
	sequence uint64
	plan     *services.MachineUnlinkPlan
	err      error
}

type detailUnlinkResultMsg struct {
	sequence uint64
	err      error
}

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
		}
	}}
}

func (m *model) openInstallationLocationActions(entry *common.Entry, ref string, item common.InstallationObservation, seq uint64) {
	location := installationLocation(item)
	items := []pages.PickerItem{{Label: "View location", Value: "view"}}
	if item.Status == "installed" && item.Skill != "" {
		items = append(items, pages.PickerItem{Label: "Preview unlink", Value: "unlink"})
	}
	state := m.installState(entry.Path, item.Target)
	if entry.Status == common.StatusActive && item.Directory == "" && (state == common.InstallConflict || state == common.InstallDangling) {
		items = append(items, pages.PickerItem{Label: "Preview repair", Value: "repair"})
	}
	m.picker = &pages.Picker{Title: location, Single: true, Hint: "enter select  esc back", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || !m.showDetail || m.detailMachineSequence != seq {
			return
		}
		switch selected[0].Value {
		case "view":
			text := location + " · " + item.Status
			if item.Path != "" {
				text += " · " + item.Path
			}
			m.setStatus(text)
		case "unlink":
			svc := m.svc
			m.setStatus("Checking link and declaration coverage…")
			m.machinePending = func() tea.Msg {
				plan, err := svc.PlanMachineUnlink(item.Directory, item.Target, item.Path)
				return detailUnlinkPreviewMsg{sequence: seq, plan: plan, err: err}
			}
		case "repair":
			m.prepareSelectedFix(entry, ref, []string{item.Target})
		}
	}}
}

func (m *model) acceptDetailUnlinkPreview(msg detailUnlinkPreviewMsg) {
	if !m.showDetail || m.detailMachineSequence != msg.sequence || m.picker != nil || m.confirm != nil || m.machine.open {
		return
	}
	if msg.err != nil {
		m.setStatus(msg.err.Error())
		return
	}
	if msg.plan == nil {
		return
	}
	plan := *msg.plan
	m.confirm = &pages.Confirm{Prompt: unlinkPrompt(plan), OnYes: func() {
		if !m.showDetail || m.detailMachineSequence != msg.sequence {
			return
		}
		ctx, svc := m.ctx, m.svc
		m.machinePending = func() tea.Msg {
			return detailUnlinkResultMsg{sequence: msg.sequence, err: svc.ApplyMachineUnlink(ctx, plan)}
		}
	}}
}

func unlinkPrompt(plan services.MachineUnlinkPlan) string {
	return fmt.Sprintf("Unlink %s from %s at %s?", plan.Skill, plan.Target, plan.Path)
}
