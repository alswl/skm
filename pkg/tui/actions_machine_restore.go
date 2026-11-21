package tui

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type machineRestorePlanMsg struct {
	sequence uint64
	plan     *services.MachineRestorePlan
	err      error
}
type machineRestoreDoneMsg struct {
	sequence uint64
	result   *services.MachineBatchResult
	err      error
}

func (m *model) openMachineRestore() {
	m.requestMachineRestore(false)
}

func (m *model) requestMachineRestore(allow bool) {
	svc := m.svc
	m.queueMachinePreview(func(seq uint64) tea.Msg {
		plan, err := svc.PlanMachineRestore(allow)
		return machineRestorePlanMsg{sequence: seq, plan: plan, err: err}
	})
}

func (m *model) acceptMachineRestorePlan(msg machineRestorePlanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	if msg.plan == nil {
		m.machine.err = fmt.Errorf("empty restore preview")
		return
	}
	plan := *msg.plan
	seen := map[string]bool{}
	var detail strings.Builder
	count, blocked, gitVisible := 0, 0, 0
	for _, batch := range plan.Plans {
		for _, item := range batch.Items {
			key := item.Path + "\x00" + item.Skill
			if item.Path == "" {
				key = item.Directory + "\x00" + item.Target + "\x00" + item.Skill
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			count++
			if item.Status == "unignored" {
				gitVisible++
			}
			if item.Status != "installed" && item.Status != "install" && (item.Status != "unignored" || !plan.Request.AllowUnignored) {
				blocked++
			}
			fmt.Fprintf(&detail, "%s  %s  %s  %s\n", installationPreviewStatus(item.Status, plan.Request.AllowUnignored), item.Target, item.Skill, item.Path)
		}
	}
	undeclared := 0
	for _, item := range plan.Observations {
		status := item.Status
		if status == "installed" {
			status = "undeclared"
			undeclared++
		}
		fmt.Fprintf(&detail, "%s  %s  %s  %s\n", status, item.Target, item.Skill, item.Path)
	}
	m.confirm = &pages.Confirm{Prompt: fmt.Sprintf("Restore %d locations from %d declarations?\n%d blocked, %d undeclared", count, len(plan.Plans), blocked, undeclared), Diff: detail.String(), OnYes: func() {
		if !m.machine.open || msg.sequence != m.machine.sequence {
			return
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			result, err := svc.ApplyMachineRestore(ctx, plan)
			return machineRestoreDoneMsg{sequence: seq, result: result, err: err}
		}
	}}
	if gitVisible > 0 && !plan.Request.AllowUnignored {
		m.confirm.Prompt += "\nSome links would appear in git status and are skipped."
		m.confirm.OnIncludeGitVisible = func() {
			if m.machine.open && msg.sequence == m.machine.sequence {
				m.requestMachineRestore(true)
			}
		}
	}
}

func (m *model) acceptMachineRestoreDone(msg machineRestoreDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	if msg.result == nil {
		m.machine.err = fmt.Errorf("empty restore result")
		return nil
	}
	var out strings.Builder
	partial := false
	for _, item := range msg.result.Items {
		fmt.Fprintf(&out, "%s  %s  %s  %s", item.Directory, item.Target, item.Skill, item.Status)
		if item.Error != "" {
			fmt.Fprintf(&out, ": %s", item.Error)
		}
		if item.Status != "installed" {
			partial = true
		}
		out.WriteByte('\n')
	}
	m.machine.result = strings.TrimSpace(out.String())
	retry := m.machine.retryPreview
	cmd := m.startMachineScan()
	if partial && retry != nil {
		m.machine.retryPreview = retry
		m.machine.result += "\nPress v to preview the remaining work again"
	}
	return cmd
}
