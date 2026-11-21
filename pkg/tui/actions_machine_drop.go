package tui

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type machineDeclarationsMsg struct {
	sequence uint64
	items    []common.MachineInstall
	err      error
}
type machineDropPlanMsg struct {
	sequence uint64
	plan     *services.MachineDropPlan
	err      error
}
type machineDropDoneMsg struct {
	sequence uint64
	err      error
}

func (m *model) startMachineDrop() tea.Cmd {
	m.machine.sequence++
	m.machine.scanning = true
	seq, svc := m.machine.sequence, m.svc
	return func() tea.Msg {
		items, err := svc.ListMachineDeclarations()
		return machineDeclarationsMsg{sequence: seq, items: items, err: err}
	}
}

func (m *model) acceptMachineDeclarations(msg machineDeclarationsMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	items := make([]pages.PickerItem, 0, len(msg.items))
	for i, install := range msg.items {
		scope := install.Scope.Kind
		if install.Scope.Name != "" {
			scope += ":" + install.Scope.Name
		}
		if install.Scope.Kind == "tag" {
			count := 0
			for _, dir := range m.machine.allDirectories {
				for _, tag := range dir.Tags {
					if tag == install.Scope.Name {
						count++
						break
					}
				}
			}
			scope += fmt.Sprintf(" (%d directories)", count)
		}
		items = append(items, pages.PickerItem{Label: fmt.Sprintf("%s  %s  %s", install.Skill, install.Target, scope), Value: fmt.Sprint(i)})
	}
	if len(items) == 0 {
		m.setStatus("no machine declarations")
		return
	}
	m.picker = &pages.Picker{Title: "drop declaration", Hint: "enter select  esc cancel", Single: true, Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || !m.machine.open || msg.sequence != m.machine.sequence {
			return
		}
		var index int
		if _, err := fmt.Sscan(selected[0].Value, &index); err != nil || index < 0 || index >= len(msg.items) {
			return
		}
		install := msg.items[index]
		svc := m.svc
		m.queueMachinePreview(func(seq uint64) tea.Msg {
			plan, err := svc.PlanMachineDrop(install)
			return machineDropPlanMsg{sequence: seq, plan: plan, err: err}
		})
	}}
}

func (m *model) acceptMachineDropPlan(msg machineDropPlanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	if msg.plan == nil {
		m.machine.err = fmt.Errorf("empty drop preview")
		return
	}
	plan := *msg.plan
	m.confirm = &pages.Confirm{Prompt: fmt.Sprintf("Drop %s / %s / %s:%s? Links stay in place.", plan.Install.Skill, plan.Install.Target, plan.Install.Scope.Kind, plan.Install.Scope.Name), OnYes: func() {
		if !m.machine.open || msg.sequence != m.machine.sequence {
			return
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			err := svc.ApplyMachineDrop(ctx, plan)
			return machineDropDoneMsg{sequence: seq, err: err}
		}
	}}
}

func (m *model) acceptMachineDropDone(msg machineDropDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	m.setStatus("declaration dropped; links retained")
	return m.startMachineScan()
}
