package tui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	tea "github.com/charmbracelet/bubbletea"
)

type tagCatalogMsg struct {
	request      uint64
	tags         []string
	dirs         []services.MachineDirectoryInfo
	declarations []common.Installation
	initial      string
	restrict     []string
	returnTo     func()
	err          error
}

type tagMutationMsg struct {
	request uint64
	catalog tagCatalogMsg
	prompt  string
	diff    string
	apply   func(context.Context) error
	err     error
}

type tagMutationDoneMsg struct {
	request uint64
	catalog tagCatalogMsg
	err     error
}

func (m *model) openTagManagement(initial string, restrict []string, onReturn func()) {
	m.tagViewRequest++
	seq, svc := m.tagViewRequest, m.svc
	m.picker = &pages.Picker{Title: "Tag management", Prelude: "Loading tags…", Single: true, OnCancel: func() { m.tagViewRequest++ }}
	m.machinePending = func() tea.Msg {
		msg := tagCatalogMsg{request: seq, initial: initial, returnTo: onReturn, restrict: restrict}
		msg.dirs, msg.err = svc.ListDirectories()
		if msg.err != nil {
			return msg
		}
		msg.declarations, msg.err = svc.ListMachineDeclarations()
		if msg.err != nil {
			return msg
		}
		if restrict != nil {
			msg.tags = slices.Clone(restrict)
		} else {
			for _, d := range msg.dirs {
				msg.tags = append(msg.tags, d.Tags...)
			}
			for _, declaration := range msg.declarations {
				if declaration.Scope.Kind == "tag" {
					msg.tags = append(msg.tags, declaration.Scope.Name)
				}
			}
		}
		slices.Sort(msg.tags)
		msg.tags = slices.Compact(msg.tags)
		return msg
	}
}

func (m *model) acceptTagCatalog(msg tagCatalogMsg) {
	if msg.request != m.tagViewRequest {
		return
	}
	if msg.err != nil {
		m.picker = nil
		m.setStatus(msg.err.Error())
		return
	}
	if msg.initial != "" && slices.Contains(msg.tags, msg.initial) {
		m.showTag(msg, msg.initial)
	} else {
		m.showTagList(msg)
	}
}

func (m *model) showTagList(msg tagCatalogMsg) {
	items := make([]pages.PickerItem, 0, len(msg.tags))
	for _, tag := range msg.tags {
		items = append(items, pages.PickerItem{Label: tag, Value: tag})
	}
	prelude := ""
	if len(items) == 0 {
		prelude = "No tags. Open Directories to register or tag a directory."
	}
	m.picker = &pages.Picker{Title: "Tags", Prelude: prelude, Single: true, Items: items, Hint: "enter view  esc back",
		OnCancel: msg.returnTo,
		OnConfirm: func(selected []pages.PickerItem) {
			if len(selected) != 0 {
				m.showTag(msg, selected[0].Value)
			}
		},
	}
}

func (m *model) showTag(msg tagCatalogMsg, tag string) {
	members := []string{}
	items := []pages.PickerItem{{Label: "Edit members", Value: "edit"}}
	for i, declaration := range msg.declarations {
		if declaration.Scope.Kind == "tag" && declaration.Scope.Name == tag {
			items = append(items, pages.PickerItem{Label: "Declaration · " + installationDeclarationLabel(declaration), Value: strconv.Itoa(i)})
		}
	}
	for _, dir := range msg.dirs {
		if slices.Contains(dir.Tags, tag) {
			members = append(members, dir.ID)
			label := "Member · " + dir.ID
			if dir.Path == "" {
				label += " · missing local path"
			}
			items = append(items, pages.PickerItem{Label: label, Value: "member"})
		}
	}
	m.picker = &pages.Picker{Title: "Tag · " + tag, Prelude: fmt.Sprintf("%d member directories", len(members)), Items: items, Single: true, Hint: "enter select  esc back",
		OnCancel: func() { m.showTagList(msg) },
		OnConfirm: func(selected []pages.PickerItem) {
			if len(selected) == 0 {
				return
			}
			switch selected[0].Value {
			case "edit":
				m.editTagMembers(msg, tag)
			case "member":
				m.showTag(msg, tag)
			default:
				index, err := strconv.Atoi(selected[0].Value)
				if err == nil && index >= 0 && index < len(msg.declarations) {
					m.showTagDeclaration(msg, tag, msg.declarations[index])
				}
			}
		},
	}
}

func (m *model) editTagMembers(msg tagCatalogMsg, tag string) {
	items := make([]pages.PickerItem, 0, len(msg.dirs))
	for _, dir := range msg.dirs {
		items = append(items, pages.PickerItem{Label: dir.ID, Value: dir.ID, Checked: slices.Contains(dir.Tags, tag)})
	}
	m.picker = &pages.Picker{Title: "Tag members · " + tag, Items: items, Hint: "space select  enter preview  esc back",
		OnCancel: func() { m.showTag(msg, tag) },
		OnConfirm: func(selected []pages.PickerItem) {
			next := pickerValues(selected)
			m.previewTagChange(msg, tag, func() tagMutationMsg {
				svc := m.svc
				plan, err := svc.PlanMachineTagMembers(tag, next)
				change := tagMutationMsg{err: err}
				if err == nil {
					change.prompt = fmt.Sprintf("Change %s members?\nBefore: %s\nAfter: %s\n%d affected declarations. Links stay in place.", tag, memberList(plan.Before), memberList(plan.After), len(plan.Declarations))
					var lines []string
					for _, item := range plan.Declarations {
						lines = append(lines, installationDeclarationLabel(item))
					}
					change.diff = strings.Join(lines, "\n")
					change.apply = func(ctx context.Context) error { return svc.ApplyMachineTagMembers(ctx, *plan) }
				}
				return change
			})
		},
	}
}

func memberList(members []string) string {
	if len(members) == 0 {
		return "none"
	}
	return strings.Join(members, ", ")
}

func (m *model) showTagDeclaration(msg tagCatalogMsg, tag string, declaration common.Installation) {
	m.picker = &pages.Picker{Title: "installation declaration", Prelude: installationDeclarationLabel(declaration), Single: true, Hint: "enter preview  esc back",
		Items:    []pages.PickerItem{{Label: "Drop this declaration", Value: "drop"}},
		OnCancel: func() { m.showTag(msg, tag) },
		OnConfirm: func(selected []pages.PickerItem) {
			if len(selected) == 0 {
				return
			}
			m.previewTagChange(msg, tag, func() tagMutationMsg {
				svc := m.svc
				plan, err := svc.PlanMachineDrop(declaration)
				change := tagMutationMsg{err: err}
				if err == nil {
					change.prompt = "Drop " + installationDeclarationLabel(declaration) + "? Links stay in place."
					change.apply = func(ctx context.Context) error { return svc.ApplyMachineDrop(ctx, *plan) }
				}
				return change
			})
		},
	}
}

func (m *model) previewTagChange(catalog tagCatalogMsg, tag string, prepare func() tagMutationMsg) {
	catalog.initial = tag
	m.tagViewRequest++
	seq := m.tagViewRequest
	m.picker = &pages.Picker{Title: "Tag · " + tag, Prelude: "Preparing preview…", Single: true, OnCancel: func() { m.tagViewRequest++; m.showTag(catalog, tag) }}
	m.machinePending = func() tea.Msg {
		msg := prepare()
		msg.request, msg.catalog = seq, catalog
		return msg
	}
}

func (m *model) acceptTagMutation(msg tagMutationMsg) {
	if msg.request != m.tagViewRequest {
		return
	}
	m.picker = nil
	if msg.err != nil {
		m.setStatus(msg.err.Error())
		m.showTag(msg.catalog, msg.catalog.initial)
		return
	}
	m.confirm = &pages.Confirm{Prompt: msg.prompt, Diff: msg.diff, OnCancel: func() { m.showTag(msg.catalog, msg.catalog.initial) }, OnYes: func() {
		ctx := m.ctx
		pending := &pages.Picker{Title: "Tag · " + msg.catalog.initial, Prelude: "Applying reviewed change…", Single: true}
		pending.OnCancel = func() { m.picker = pending }
		m.picker = pending
		m.machinePending = func() tea.Msg {
			return tagMutationDoneMsg{request: msg.request, catalog: msg.catalog, err: msg.apply(ctx)}
		}
	}}
}

func (m *model) acceptTagMutationDone(msg tagMutationDoneMsg) tea.Cmd {
	if msg.request != m.tagViewRequest {
		return nil
	}
	if msg.err != nil {
		m.setStatus(msg.err.Error())
	} else {
		m.setStatus("Tag updated; links retained")
	}
	m.openTagManagement(msg.catalog.initial, msg.catalog.restrict, msg.catalog.returnTo)
	return scanCmd(m.svc, scanAfterJob, 0)
}
