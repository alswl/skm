package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type machineInstallPlanMsg struct {
	sequence uint64
	plan     *services.MachineBatchPlan
	err      error
}

type machineInstallDoneMsg struct {
	sequence uint64
	result   *services.MachineBatchResult
	err      error
}

func (m *model) openMachineInstallSkills() {
	items := []pages.PickerItem{}
	for _, entry := range m.entries {
		if entry.Kind == common.KindSkill && entry.Status == common.StatusActive {
			items = append(items, pages.PickerItem{Label: entry.Name, Value: m.svc.Repo.RelPath(entry.Path)})
		}
	}
	if len(items) == 0 {
		m.setStatus("no active skills")
		return
	}
	m.picker = &pages.Picker{Title: "select skills", Hint: "space select  enter next  esc cancel", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.setStatus("no skills selected")
			return
		}
		m.openMachineInstallScope(pickerValues(selected))
	}}
}

func (m *model) openMachineInstallScope(skills []string) {
	items := []pages.PickerItem{{Label: "user", Value: "user"}, {Label: "directory tags", Value: "tag"}}
	if m.machine.cursor > 0 {
		items = append(items, pages.PickerItem{Label: "directory " + m.machineScope(), Value: "directory"})
	}
	m.picker = &pages.Picker{Title: "installation scope", Hint: "enter select  esc cancel", Single: true, Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			return
		}
		request := services.MachineBatchRequest{Skills: skills}
		switch selected[0].Value {
		case "user":
			request.User = true
		case "directory":
			request.Directories = []string{m.machineScope()}
		case "tag":
			m.openMachineInstallTags(request)
			return
		default:
			return
		}
		m.openMachineInstallTargets(request)
	}}
}

func (m *model) openMachineInstallTags(request services.MachineBatchRequest) {
	counts := map[string]int{}
	for _, directory := range m.machine.allDirectories {
		for _, tag := range directory.Tags {
			counts[tag]++
		}
	}
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	items := make([]pages.PickerItem, 0, len(tags))
	for _, tag := range tags {
		items = append(items, pages.PickerItem{Label: fmt.Sprintf("%s  (%d directories)", tag, counts[tag]), Value: tag})
	}
	if len(items) == 0 {
		m.setStatus("no directory tags available")
		return
	}
	m.picker = &pages.Picker{Title: "select tags", Hint: "space select  enter next  esc cancel", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.setStatus("no tags selected")
			return
		}
		request.Tags = pickerValues(selected)
		m.openMachineInstallTargets(request)
	}}
}

func (m *model) openMachineInstallTargets(request services.MachineBatchRequest) {
	items := []pages.PickerItem{}
	if request.User {
		for _, target := range m.svc.Cfg.Targets {
			strategy, ok := target.EffectiveStrategy(common.KindSkill)
			if target.AcceptsKind(common.KindSkill) && ok && strategy == common.StrategySkillSymlink {
				items = append(items, pages.PickerItem{Label: target.Name, Value: target.Name})
			}
		}
	} else {
		items = []pages.PickerItem{{Label: "Codex", Value: "codex"}, {Label: "Claude Code", Value: "claude"}}
	}
	if len(items) == 0 {
		m.setStatus("no suitable targets")
		return
	}
	m.picker = &pages.Picker{Title: "select clients", Hint: "space select  enter preview  esc cancel", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.setStatus("no clients selected")
			return
		}
		request.Targets = pickerValues(selected)
		if request.User {
			m.requestMachineInstallPlan(request)
			return
		}
		m.picker = &pages.Picker{Title: "unignored project links", Hint: "enter select  esc cancel", Single: true, Items: []pages.PickerItem{{Label: "Block unignored links", Value: "block"}, {Label: "Allow unignored links", Value: "allow"}}, OnConfirm: func(choice []pages.PickerItem) {
			if len(choice) == 0 {
				return
			}
			request.AllowUnignored = choice[0].Value == "allow"
			m.requestMachineInstallPlan(request)
		}}
	}}
}

func (m *model) requestMachineInstallPlan(request services.MachineBatchRequest) {
	svc := m.svc
	m.queueMachinePreview(func(seq uint64) tea.Msg {
		plan, err := svc.PlanMachineBatch(request)
		return machineInstallPlanMsg{sequence: seq, plan: plan, err: err}
	})
}

func (m *model) acceptMachineInstallPlan(msg machineInstallPlanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	if msg.plan == nil {
		m.machine.err = fmt.Errorf("empty installation preview")
		return
	}
	plan := *msg.plan
	if !plan.Request.User && len(plan.Directories) == 0 {
		m.machine.err = fmt.Errorf("no directories match the selected scope")
		return
	}
	counts := map[string]int{}
	var detail strings.Builder
	for _, item := range plan.Items {
		counts[item.Status]++
		fmt.Fprintf(&detail, "%s  %s  %s  %s\n", item.Status, item.Target, item.Skill, item.Path)
	}
	installable := counts["install"]
	if plan.Request.AllowUnignored {
		installable += counts["unignored"]
	}
	scope := "user"
	if len(plan.Request.Tags) != 0 {
		scope = "tags " + strings.Join(plan.Request.Tags, ", ")
	} else if len(plan.Request.Directories) != 0 {
		scope = "directories " + strings.Join(plan.Request.Directories, ", ")
	}
	prompt := fmt.Sprintf("Apply %d locations across %d directories (%s)? install %d, already installed %d, blocked %d", len(plan.Items), len(plan.Directories), scope, installable, counts["installed"], len(plan.Items)-installable-counts["installed"])
	m.confirm = &pages.Confirm{Prompt: prompt, Diff: detail.String(), OnYes: func() {
		if !m.machine.open || msg.sequence != m.machine.sequence {
			return
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			result, err := svc.ApplyMachineBatch(ctx, plan)
			return machineInstallDoneMsg{sequence: seq, result: result, err: err}
		}
	}}
}

func (m *model) acceptMachineInstallDone(msg machineInstallDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	if msg.result == nil {
		m.machine.err = fmt.Errorf("empty installation result")
		return nil
	}
	var result strings.Builder
	partial := false
	for _, item := range msg.result.Items {
		fmt.Fprintf(&result, "%s  %s  %s  %s", item.Directory, item.Target, item.Skill, item.Status)
		if item.Error != "" {
			fmt.Fprintf(&result, ": %s", item.Error)
		}
		if item.Status != "installed" {
			partial = true
		}
		result.WriteByte('\n')
	}
	m.machine.result = strings.TrimSpace(result.String())
	retry := m.machine.retryPreview
	cmd := m.startMachineScan()
	if partial && retry != nil {
		m.machine.retryPreview = retry
		m.machine.result += "\nPress v to preview the remaining work again"
	}
	return cmd
}
