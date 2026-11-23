package tui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type installationLocationState struct {
	item    common.InstallationObservation
	back    func()
	refresh func() tea.Cmd
	repair  func()
}

type locationCoverageMsg struct {
	location *installationLocationState
	items    []common.Installation
	err      error
}

type locationPreviewMsg struct {
	location *installationLocationState
	prompt   string
	diff     string
	success  string
	apply    func(context.Context) error
	onCancel func()
	request  uint64
	unlinked bool
	err      error
}

type locationAppliedMsg struct {
	location *installationLocationState
	success  string
	unlinked bool
	err      error
}

func (m *model) openLocation(location *installationLocationState) {
	m.location = location
	m.picker = &pages.Picker{Title: "installation location", Prelude: "Checking declaration coverage…", Single: true, OnCancel: func() {
		m.location = nil
		if location.back != nil {
			location.back()
		}
	}}
	svc := m.svc
	m.machinePending = func() tea.Msg {
		items, err := svc.MachineLocationCoverage(location.item.Path)
		return locationCoverageMsg{location: location, items: items, err: err}
	}
}

func installationDeclarationLabel(item common.Installation) string {
	return strings.TrimSpace(fmt.Sprintf("%s / %s · %s %s", item.Skill, item.Target, item.Scope.Kind, item.Scope.Name))
}

func (m *model) acceptLocationCoverage(msg locationCoverageMsg) {
	location := msg.location
	if m.location != location {
		return
	}
	item := location.item
	title := "installation location · " + installationLocation(item)
	prelude := fmt.Sprintf("%s · %s\n%s", item.Skill, item.Status, item.Path)
	if item.Error != "" {
		prelude += "\n" + item.Error
	}
	switch {
	case msg.err != nil:
		prelude += "\nCoverage could not be fully verified: " + strings.ReplaceAll(msg.err.Error(), "\n", " · ")
	case len(msg.items) > 0:
		prelude += fmt.Sprintf("\n%d covering declarations · selected link remains covered by a declaration", len(msg.items))
	default:
		prelude += "\nNo covering declarations"
	}
	options := []pages.PickerItem{{Label: "View location", Value: "view"}}
	for i, declaration := range msg.items {
		options = append(options, pages.PickerItem{Label: "View declaration · " + installationDeclarationLabel(declaration), Value: strconv.Itoa(i)})
	}
	if len(msg.items) == 0 && msg.err == nil && item.Status == "installed" && item.Skill != "" {
		options = append(options, pages.PickerItem{Label: "Preview unlink", Value: "unlink"})
	}
	if len(msg.items) == 0 && msg.err == nil && item.Status == "installed" && item.Skill != "" {
		options = append(options, pages.PickerItem{Label: "Capture existing installation", Value: "capture"})
	}
	if location.repair != nil {
		options = append(options, pages.PickerItem{Label: "Preview repair", Value: "repair"})
	}
	m.picker = &pages.Picker{Title: title, Prelude: prelude, Single: true, Hint: "enter select  esc back", Items: options, OnCancel: func() {
		m.location = nil
		if location.back != nil {
			location.back()
		}
	}, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || m.location != location {
			return
		}
		switch selected[0].Value {
		case "view":
			m.acceptLocationCoverage(msg)
			m.setStatus(installationLocation(item) + " · " + item.Status + " · " + item.Path)
		case "capture":
			m.previewLocationCapture(location)
		case "unlink":
			m.previewLocationUnlink(location)
		case "repair":
			m.location = nil
			location.repair()
		default:
			index, err := strconv.Atoi(selected[0].Value)
			if err == nil && index >= 0 && index < len(msg.items) {
				m.openLocationDeclaration(msg, msg.items[index])
			}
		}
	}}
}

func (m *model) previewLocationCapture(location *installationLocationState) {
	m.locationRequest++
	request := m.locationRequest
	m.locationBusy("Preparing preview…", func() { m.openLocation(location) })
	svc, item := m.svc, location.item
	m.machinePending = func() tea.Msg {
		result := locationPreviewMsg{location: location, request: request, success: "installation captured"}
		plan, err := svc.PlanMachineCapture(item.Directory, item.Target, item.Path)
		result.err = err
		if err == nil {
			result.prompt = fmt.Sprintf("Capture %s for %s at %s? Existing link is kept.", plan.Install.Skill, plan.Target, plan.Path)
			result.apply = func(ctx context.Context) error { return svc.ApplyMachineCapture(ctx, *plan) }
		}
		return result
	}
}

func (m *model) previewLocationUnlink(location *installationLocationState) {
	m.locationRequest++
	request := m.locationRequest
	m.locationBusy("Preparing preview…", func() { m.openLocation(location) })
	svc, item := m.svc, location.item
	m.machinePending = func() tea.Msg {
		result := locationPreviewMsg{location: location, request: request, success: "link removed", unlinked: true}
		plan, err := svc.PlanMachineUnlink(item.Directory, item.Target, item.Path)
		result.err = err
		if err == nil {
			result.prompt = unlinkPrompt(*plan)
			result.apply = func(ctx context.Context) error { return svc.ApplyMachineUnlink(ctx, *plan) }
		}
		return result
	}
}

func (m *model) openLocationDeclaration(coverage locationCoverageMsg, declaration common.Installation) {
	location := coverage.location
	items := []pages.PickerItem{{Label: "Drop this declaration", Value: "drop"}}
	if declaration.Scope.Kind == "tag" && location.item.Directory != "" {
		items = append([]pages.PickerItem{{Label: "Exclude current directory", Value: "exit"}}, items...)
	}
	m.picker = &pages.Picker{Title: "installation declaration", Prelude: installationDeclarationLabel(declaration), Hint: "enter preview  esc back", Single: true, Items: items, OnCancel: func() {
		m.acceptLocationCoverage(coverage)
	}, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) > 0 && m.location == location {
			m.previewLocationDeclaration(coverage, declaration, selected[0].Value == "exit")
		}
	}}
}

func (m *model) previewLocationDeclaration(coverage locationCoverageMsg, declaration common.Installation, exit bool) {
	location := coverage.location
	back := func() { m.openLocationDeclaration(coverage, declaration) }
	m.locationRequest++
	request := m.locationRequest
	m.locationBusy("Preparing preview…", back)
	svc := m.svc
	m.machinePending = func() tea.Msg {
		result := locationPreviewMsg{location: location, request: request, onCancel: back}
		if exit {
			plan, err := svc.PlanMachineTagExit(declaration, location.item.Directory, location.item.Path)
			result.err = err
			if err != nil {
				return result
			}
			keep := "none (drop this declaration)"
			if len(plan.Remaining) > 0 {
				keep = strings.Join(plan.Remaining, ", ")
			}
			result.prompt = fmt.Sprintf("Change %s?\nExclude: %s\nKeep: %s\nFuture tag members require an explicit directory declaration.\nLinks stay in place.", installationDeclarationLabel(declaration), plan.Directory, keep)
			var coverage []string
			for _, item := range plan.Coverage {
				coverage = append(coverage, installationDeclarationLabel(item))
			}
			result.prompt += fmt.Sprintf("\n%d other declarations cover this slot.", len(coverage))
			if plan.CoverageGap != "" {
				coverage = append(coverage, plan.CoverageGap)
				result.prompt += " Some sources could not be verified."
			}
			result.diff = strings.Join(coverage, "\n")
			result.success = "declaration changed; links retained"
			result.apply = func(ctx context.Context) error { return svc.ApplyMachineTagExit(ctx, *plan) }
		} else {
			plan, err := svc.PlanMachineDrop(declaration)
			result.err = err
			if err != nil {
				return result
			}
			result.prompt = "Drop " + installationDeclarationLabel(declaration) + "? Links stay in place."
			result.success = "declaration dropped; links retained"
			result.apply = func(ctx context.Context) error { return svc.ApplyMachineDrop(ctx, *plan) }
		}
		return result
	}
}

func (m *model) acceptLocationPreview(msg locationPreviewMsg) {
	if m.location != msg.location || m.locationRequest != msg.request {
		return
	}
	m.picker = nil
	back := msg.onCancel
	if back == nil {
		back = func() { m.openLocation(msg.location) }
	}
	if msg.err != nil {
		m.setStatus(msg.err.Error())
		back()
		return
	}
	m.confirm = &pages.Confirm{Prompt: msg.prompt, Diff: msg.diff, OnCancel: back, OnYes: func() {
		if m.location == msg.location {
			ctx := m.ctx
			m.locationBusy("Applying reviewed change…", nil)
			m.machinePending = func() tea.Msg {
				return locationAppliedMsg{location: msg.location, success: msg.success, unlinked: msg.unlinked, err: msg.apply(ctx)}
			}
		}
	}}
}

func (m *model) acceptLocationApplied(msg locationAppliedMsg) tea.Cmd {
	if m.location != msg.location {
		return nil
	}
	location := msg.location
	m.picker = nil
	var refresh tea.Cmd
	if location.refresh != nil {
		refresh = location.refresh()
	}
	if msg.err != nil {
		m.setStatus(msg.err.Error())
		m.openLocation(location)
		return refresh
	}
	m.setStatus(msg.success)
	if msg.unlinked {
		m.location = nil
	} else {
		m.openLocation(location)
	}
	return tea.Batch(refresh, scanCmd(m.svc, scanAfterJob, 0))
}

func (m *model) locationBusy(message string, cancel func()) {
	picker := &pages.Picker{Title: "installation location", Prelude: message, Single: true}
	picker.OnConfirm = func([]pages.PickerItem) { m.picker = picker }
	picker.OnCancel = func() {
		if cancel != nil {
			m.locationRequest++
			cancel()
		} else {
			m.picker = picker
		}
	}
	m.picker = picker
}

func locationResultIndex(items []common.InstallationObservation, selected common.InstallationObservation, oldIndex int) int {
	index := slices.IndexFunc(items, func(item common.InstallationObservation) bool {
		return item.Target == selected.Target && item.Path == selected.Path
	})
	if index >= 0 {
		return index
	}
	return clampInt(oldIndex, 0, maxInt(0, len(items)-1))
}
