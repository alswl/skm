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
	cursor := 0
	if m.machine.cursor > 0 {
		items = append(items, pages.PickerItem{Label: "directory " + m.machineScope(), Value: "directory"})
		cursor = 2
	}
	if len(m.machine.allDirectories) > 0 {
		items = append(items, pages.PickerItem{Label: "choose directory", Value: "choose-directory"})
	}
	if m.machine.filter != "" && !m.machine.installFromSkill {
		cursor = 1
	}
	if m.machine.installFromSkill && m.cursor < len(m.filtered) {
		for _, target := range m.svc.Cfg.Targets {
			if strategy, ok := target.EffectiveStrategy(common.KindSkill); ok && target.AcceptsKind(common.KindSkill) && strategy != common.StrategySkillSymlink {
				items = append(items, pages.PickerItem{Label: "custom user installs", Value: "custom"})
				break
			}
		}
	}
	m.picker = &pages.Picker{Title: "installation scope", Hint: "enter select  esc cancel", Single: true, Items: items, Cursor: cursor, OnCancel: m.cancelSkillInstall, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			return
		}
		request := services.MachineBatchRequest{Skills: skills}
		switch selected[0].Value {
		case "user":
			request.User = true
			request.LocalOnly = m.svc.Cfg.UserSkillInstallLocalOnly
		case "directory":
			request.Directories = []string{m.machineScope()}
		case "choose-directory":
			m.openInstallationDirectoryPicker(request)
			return
		case "custom":
			entry := m.filtered[m.cursor]
			m.cancelSkillInstall()
			m.openInstallsPicker(entry)
			return
		case "tag":
			m.openMachineInstallTags(request)
			return
		default:
			return
		}
		m.openMachineInstallTargets(request)
	}}
}

func (m *model) openInstallationDirectoryPicker(request services.MachineBatchRequest) {
	items := make([]pages.PickerItem, 0, len(m.machine.allDirectories))
	for _, directory := range m.machine.allDirectories {
		items = append(items, pages.PickerItem{Label: directory.ID + "  " + strings.Join(directory.Tags, ", "), Value: directory.ID})
	}
	m.picker = &pages.Picker{Title: "select directory", Searchable: true, Hint: "enter select  / search  esc cancel", Single: true, Items: items, OnCancel: m.cancelSkillInstall, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			return
		}
		request.Directories = pickerValues(selected)
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
	cursor := 0
	for _, tag := range tags {
		checked := !m.machine.installFromSkill && tag == m.machine.filter
		if checked {
			cursor = len(items)
		}
		items = append(items, pages.PickerItem{Label: fmt.Sprintf("%s  (%d directories)", tag, counts[tag]), Value: tag, Checked: checked})
	}
	if len(items) == 0 {
		m.openMachineInstallScope(request.Skills)
		m.setStatus("no directory tags available")
		return
	}
	m.picker = &pages.Picker{Title: "select tags", Hint: "space select  enter next  esc cancel", Items: items, Cursor: cursor, OnCancel: m.cancelSkillInstall, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.openMachineInstallTags(request)
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
		m.cancelSkillInstall()
		m.setStatus("no suitable targets")
		return
	}
	if len(items) == 1 {
		items[0].Checked = true
	}
	m.picker = &pages.Picker{Title: "select clients", OnCancel: m.cancelSkillInstall, Hint: "space select  enter preview  esc cancel", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.openMachineInstallTargets(request)
			m.setStatus("no clients selected")
			return
		}
		request.Targets = pickerValues(selected)
		m.requestMachineInstallPlan(request)
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
		fmt.Fprintf(&detail, "%s  %s  %s  %s\n", installationPreviewStatus(item.Status, plan.Request.AllowUnignored), item.Target, item.Skill, item.Path)
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
	prompt := fmt.Sprintf("Apply %d locations across %d directories (%s)?\nInstall %d · already installed %d · blocked %d", len(plan.Items), len(plan.Directories), scope, installable, counts["installed"], len(plan.Items)-installable-counts["installed"])
	declared := 0
	for _, d := range plan.Declarations {
		if !d.Exists {
			declared++
		}
	}
	if plan.Request.LocalOnly {
		prompt += "\nLocal only · no declarations will be written"
	} else {
		prompt += fmt.Sprintf("\nNew installation declarations: %d", declared)
	}
	m.confirm = &pages.Confirm{Prompt: prompt, Diff: detail.String(), OnCancel: m.cancelSkillInstall, OnYes: func() {
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
	if plan.Request.User {
		m.confirm.OnToggleDeclaration = func() {
			if m.machine.open && msg.sequence == m.machine.sequence {
				request := plan.Request
				request.LocalOnly = !request.LocalOnly
				m.requestMachineInstallPlan(request)
			}
		}
	}
	if counts["unignored"] > 0 && !plan.Request.AllowUnignored {
		m.confirm.Prompt += "\nSome links would appear in git status and are skipped."
		m.confirm.OnIncludeGitVisible = func() {
			if m.machine.open && msg.sequence == m.machine.sequence {
				request := plan.Request
				request.AllowUnignored = true
				m.requestMachineInstallPlan(request)
			}
		}
	}
}

func installationPreviewStatus(status string, allow bool) string {
	if status != "unignored" {
		return status
	}
	if allow {
		return "install (visible in git status)"
	}
	return "blocked (visible in git status)"
}

func (m *model) canPlanSelectedSkill() bool {
	return m.cursor < len(m.filtered) && m.filtered[m.cursor].Kind == common.KindSkill && m.filtered[m.cursor].Status == common.StatusActive
}

func (m *model) planSelectedSkill() {
	if !m.canPlanSelectedSkill() {
		return
	}
	skill := m.svc.Repo.RelPath(m.filtered[m.cursor].Path)
	m.openMachineDirectories()
	m.machine.requestedSkill = skill
	m.machine.installFromSkill = true
}

func (m *model) cancelSkillInstall() {
	if m.machine.installFromSkill {
		m.machine.open = false
		m.machine.installFromSkill = false
		m.machine.sequence++
	}
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
	if m.machine.installFromSkill && !partial {
		m.cancelSkillInstall()
		m.setStatus(fmt.Sprintf("Installation complete · %d locations", len(msg.result.Items)))
		if m.showDetail {
			m.startMachineSkillScan()
		}
		return scanCmd(m.svc, scanAfterJob, 0)
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
