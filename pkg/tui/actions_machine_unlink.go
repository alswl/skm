package tui

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type machineUnlinkPlanMsg struct {
	sequence uint64
	scope    string
	plan     *services.MachineUnlinkPlan
	err      error
}

type machineUnlinkDoneMsg struct {
	sequence uint64
	scope    string
	err      error
}

func (m *model) openMachineUnlinkPicker() {
	var candidates []common.InstallationObservation
	var items []pages.PickerItem
	for _, item := range m.machine.items {
		if item.Status != "installed" || item.Skill == "" {
			continue
		}
		candidates = append(candidates, item)
		items = append(items, pages.PickerItem{Label: fmt.Sprintf("%s  %s  %s", item.Target, item.Skill, item.Path), Value: fmt.Sprint(len(candidates) - 1)})
	}
	if len(items) == 0 {
		m.setStatus("no matching library links to unlink")
		return
	}
	scope := m.machineScope()
	m.picker = &pages.Picker{Title: "unlink installation", Hint: "enter select  esc cancel", Single: true, Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || !m.machine.open || scope != m.machineScope() {
			return
		}
		var index int
		if _, err := fmt.Sscan(selected[0].Value, &index); err != nil || index < 0 || index >= len(candidates) {
			return
		}
		item := candidates[index]
		svc := m.svc
		m.queueMachinePreview(func(seq uint64) tea.Msg {
			plan, err := svc.PlanMachineUnlink(scope, item.Target, item.Path)
			return machineUnlinkPlanMsg{sequence: seq, scope: scope, plan: plan, err: err}
		})
	}}
}

func (m *model) acceptMachineUnlinkPlan(msg machineUnlinkPlanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	if msg.plan == nil {
		m.machine.err = fmt.Errorf("empty unlink preview")
		return
	}
	plan := *msg.plan
	m.confirm = &pages.Confirm{Prompt: fmt.Sprintf("Unlink %s from %s at %s?", plan.Skill, plan.Target, plan.Path), OnYes: func() {
		if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
			return
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			err := svc.ApplyMachineUnlink(ctx, plan)
			return machineUnlinkDoneMsg{sequence: seq, scope: msg.scope, err: err}
		}
	}}
}

func (m *model) acceptMachineUnlinkDone(msg machineUnlinkDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	m.setStatus("link removed")
	return m.startMachineScan()
}
