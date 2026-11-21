package tui

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type machineDirectoryState struct {
	open           bool
	sequence       uint64
	loadingList    bool
	directories    []services.MachineDirectoryInfo
	allDirectories []services.MachineDirectoryInfo
	cursor         int
	scanning       bool
	items          []common.MachineObservation
	err            error
	tagEditing     bool
	tagInput       string
	tagDirectories []string
	filterEditing  bool
	filterInput    string
	filter         string
	pathEditing    string
	pathInput      string
	requestedScope string
	result         string
	retryPreview   func()
}

type machineDirectoriesMsg struct {
	sequence    uint64
	directories []services.MachineDirectoryInfo
	err         error
}

type machineScanMsg struct {
	sequence uint64
	scope    string
	items    []common.MachineObservation
	err      error
}

type machineCapturePlanMsg struct {
	sequence uint64
	scope    string
	plan     *services.MachineCapturePlan
	err      error
}

type machineCaptureDoneMsg struct {
	sequence uint64
	scope    string
	err      error
}

type machineTagDoneMsg struct {
	sequence uint64
	scope    string
	err      error
}

func (m *model) openMachineDirectories() {
	m.machine.open = true
	m.machine.retryPreview = nil
	m.machine.sequence++
	m.machine.loadingList = true
	m.machine.err = nil
	seq, svc := m.machine.sequence, m.svc
	m.machinePending = func() tea.Msg {
		dirs, err := svc.ListDirectories()
		return machineDirectoriesMsg{sequence: seq, directories: dirs, err: err}
	}
}

func (m *model) acceptMachineDirectories(msg machineDirectoriesMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.loadingList = false
	m.machine.err = msg.err
	if msg.err != nil {
		return nil
	}
	m.machine.allDirectories = msg.directories
	m.machine.directories = filterMachineDirectories(msg.directories, m.machine.filter)
	m.machine.cursor = 0
	if m.machine.requestedScope != "" {
		for i, directory := range m.machine.directories {
			if directory.ID == m.machine.requestedScope {
				m.machine.cursor = i + 1
				break
			}
		}
		m.machine.requestedScope = ""
	}
	return m.startMachineScan()
}

func (m *model) machineScope() string {
	if m.machine.cursor == 0 {
		return ""
	}
	return m.machine.directories[m.machine.cursor-1].ID
}

func (m *model) startMachineScan() tea.Cmd {
	m.machine.sequence++
	m.machine.retryPreview = nil
	m.machine.scanning = true
	m.machine.items = nil
	m.machine.err = nil
	seq, scope, svc := m.machine.sequence, m.machineScope(), m.svc
	return func() tea.Msg {
		var items []common.MachineObservation
		var err error
		if scope == "" {
			items, err = svc.ScanMachineUser()
		} else {
			items, err = svc.ScanMachineDirectory(scope)
		}
		return machineScanMsg{sequence: seq, scope: scope, items: items, err: err}
	}
}

func (m *model) acceptMachineScan(msg machineScanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return
	}
	m.machine.scanning = false
	m.machine.items = msg.items
	m.machine.err = msg.err
}

func (m *model) handleMachineKey(msg tea.KeyMsg) tea.Cmd {
	if m.machine.pathEditing != "" {
		return m.handleMachinePathInput(msg)
	}
	if m.machine.filterEditing {
		switch {
		case key.Matches(msg, m.keys.Esc):
			m.machine.filterEditing = false
		case key.Matches(msg, m.keys.Backspace):
			runes := []rune(m.machine.filterInput)
			if len(runes) > 0 {
				m.machine.filterInput = string(runes[:len(runes)-1])
			}
		case key.Matches(msg, m.keys.Enter):
			m.machine.filterEditing = false
			m.machine.filter = strings.TrimSpace(m.machine.filterInput)
			m.machine.directories = filterMachineDirectories(m.machine.allDirectories, m.machine.filter)
			m.machine.cursor = 0
			return m.startMachineScan()
		case msg.Type == tea.KeyRunes:
			m.machine.filterInput += string(msg.Runes)
		}
		return nil
	}
	if m.machine.tagEditing {
		switch {
		case key.Matches(msg, m.keys.Esc):
			m.machine.tagEditing = false
		case key.Matches(msg, m.keys.Backspace):
			runes := []rune(m.machine.tagInput)
			if len(runes) > 0 {
				m.machine.tagInput = string(runes[:len(runes)-1])
			}
		case key.Matches(msg, m.keys.Enter):
			tag := strings.TrimSpace(m.machine.tagInput)
			m.machine.tagEditing = false
			if tag == "" {
				m.setStatus("tag: empty input")
				return nil
			}
			scope := m.machineScope()
			m.machine.sequence++
			m.machine.scanning = true
			seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
			ids := append([]string(nil), m.machine.tagDirectories...)
			return func() tea.Msg {
				var add, remove []string
				if strings.HasPrefix(tag, "-") {
					remove = []string{strings.TrimPrefix(tag, "-")}
				} else {
					add = []string{tag}
				}
				err := svc.TagDirectories(ctx, ids, add, remove)
				return machineTagDoneMsg{sequence: seq, scope: scope, err: err}
			}
		case msg.Type == tea.KeyRunes:
			m.machine.tagInput += string(msg.Runes)
		}
		return nil
	}
	switch {
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Quit):
		m.machine.open = false
		m.machine.sequence++
	case key.Matches(msg, m.keys.MoveDown):
		if !m.machine.loadingList && m.machine.cursor < len(m.machine.directories) {
			m.machine.cursor++
			return m.startMachineScan()
		}
	case key.Matches(msg, m.keys.MoveUp):
		if !m.machine.loadingList && m.machine.cursor > 0 {
			m.machine.cursor--
			return m.startMachineScan()
		}
	case key.Matches(msg, m.keys.Refresh):
		if !m.machine.loadingList {
			return m.startMachineScan()
		}
	case key.Matches(msg, m.keys.Enter):
		if !m.machine.loadingList && !m.machine.scanning && m.machine.err == nil {
			m.openMachineCapturePicker()
		}
	case msg.String() == "t":
		if !m.machine.loadingList && m.machine.cursor > 0 {
			m.machine.tagEditing, m.machine.tagInput = true, ""
			m.machine.tagDirectories = []string{m.machineScope()}
		}
	case msg.String() == "T":
		if !m.machine.loadingList {
			m.openMachineBatchTag()
		}
	case msg.String() == "f":
		if !m.machine.loadingList {
			m.machine.filterEditing, m.machine.filterInput = true, m.machine.filter
		}
	case msg.String() == "i":
		if !m.machine.loadingList {
			m.openMachineInstallSkills()
		}
	case msg.String() == "u":
		if !m.machine.loadingList && !m.machine.scanning && m.machine.err == nil {
			m.openMachineUnlinkPicker()
		}
	case msg.String() == "a":
		m.machine.pathEditing, m.machine.pathInput = "discover", ""
	case msg.String() == "p":
		if !m.machine.loadingList && m.machine.cursor > 0 {
			m.machine.pathEditing, m.machine.pathInput = "path", ""
		}
	case msg.String() == "d":
		if !m.machine.loadingList && !m.machine.scanning {
			return m.startMachineDrop()
		}
	case msg.String() == "R":
		if !m.machine.loadingList && !m.machine.scanning {
			m.openMachineRestore()
		}
	case msg.String() == "v":
		if m.machine.retryPreview != nil && !m.machine.scanning {
			m.machine.retryPreview()
		}
	}
	return nil
}

func (m model) machineView() string {
	var body strings.Builder
	if m.machine.loadingList {
		body.WriteString("Loading directories…\n")
	} else {
		if m.machine.filter != "" {
			fmt.Fprintf(&body, "Tag: %s\n", m.machine.filter)
		}
		count := len(m.machine.directories) + 1
		shown := 5
		if count < shown {
			shown = count
		}
		start := m.machine.cursor - shown/2
		if start < 0 {
			start = 0
		}
		if start+shown > count {
			start = count - shown
		}
		for i := start; i < start+shown; i++ {
			marker := "  "
			if i == m.machine.cursor {
				marker = "› "
			}
			if i == 0 {
				fmt.Fprintf(&body, "%suser\n", marker)
				continue
			}
			d := m.machine.directories[i-1]
			fmt.Fprintf(&body, "%s%s  %s\n", marker, d.ID, strings.Join(d.Tags, ", "))
		}
		if count > shown {
			fmt.Fprintf(&body, "  … %d scopes total\n", count)
		}
		body.WriteString("\n")
		if m.machine.scanning {
			body.WriteString("Scanning…\n")
		} else if m.machine.err != nil {
			fmt.Fprintf(&body, "Error: %s\n", m.machine.err)
			if m.machine.retryPreview != nil {
				body.WriteString("Press v to preview again\n")
			}
		} else if len(m.machine.items) == 0 {
			body.WriteString("No skills found\n")
		} else {
			for _, item := range m.machine.items {
				declared := ""
				if item.Skill != "" && !item.Declared {
					declared = " (undeclared)"
				}
				fmt.Fprintf(&body, "%s  %s%s  %s\n", item.Target, item.Status, declared, item.Path)
			}
		}
	}
	if m.machine.loadingList && m.machine.err != nil {
		fmt.Fprintf(&body, "Error: %s\n", m.machine.err)
	}
	if m.machine.tagEditing {
		fmt.Fprintf(&body, "\nTag (+ by default, - to remove): %s", m.machine.tagInput)
	}
	if m.machine.filterEditing {
		fmt.Fprintf(&body, "\nFilter tag (empty clears): %s", m.machine.filterInput)
	}
	if m.machine.pathEditing != "" {
		fmt.Fprintf(&body, "\nPath: %s", m.machine.pathInput)
	}
	if m.machine.result != "" {
		fmt.Fprintf(&body, "\n%s\n", m.machine.result)
	}
	return m.framedPage("machine directories", body.String(), "j/k select  f filter  a discover  p path  enter capture  i install  R restore  d drop  u unlink  t/T tag  v retry  r refresh")
}

func filterMachineDirectories(all []services.MachineDirectoryInfo, tag string) []services.MachineDirectoryInfo {
	if tag == "" {
		return append([]services.MachineDirectoryInfo(nil), all...)
	}
	var filtered []services.MachineDirectoryInfo
	for _, directory := range all {
		for _, existing := range directory.Tags {
			if existing == tag {
				filtered = append(filtered, directory)
				break
			}
		}
	}
	return filtered
}

func (m *model) openMachineBatchTag() {
	items := make([]pages.PickerItem, 0, len(m.machine.directories))
	for _, directory := range m.machine.directories {
		items = append(items, pages.PickerItem{Label: directory.ID, Value: directory.ID})
	}
	if len(items) == 0 {
		m.setStatus("no directories to tag")
		return
	}
	m.picker = &pages.Picker{Title: "select directories to tag", Hint: "space select  enter next  esc cancel", Items: items, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 {
			m.setStatus("no directories selected")
			return
		}
		m.machine.tagDirectories = pickerValues(selected)
		m.machine.tagEditing, m.machine.tagInput = true, ""
	}}
}

func (m *model) acceptMachineTagDone(msg machineTagDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	m.setStatus("tags updated")
	m.openMachineDirectories()
	// Preserve the selected directory when the refreshed list arrives.
	m.machine.requestedScope = msg.scope
	return nil
}

func (m *model) openMachineCapturePicker() {
	var candidates []common.MachineObservation
	var options []pages.PickerItem
	for _, item := range m.machine.items {
		if item.Status != "installed" || item.Skill == "" {
			continue
		}
		candidates = append(candidates, item)
		options = append(options, pages.PickerItem{Label: fmt.Sprintf("%s  %s  %s", item.Target, item.Skill, item.Path), Value: fmt.Sprint(len(candidates) - 1)})
	}
	if len(options) == 0 {
		m.setStatus("no matching library links to capture")
		return
	}
	scope := m.machineScope()
	m.picker = &pages.Picker{Title: "capture installation", Hint: "enter select  esc cancel", Single: true, Items: options, OnConfirm: func(selected []pages.PickerItem) {
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
			plan, err := svc.PlanMachineCapture(scope, item.Target, item.Path)
			return machineCapturePlanMsg{sequence: seq, scope: scope, plan: plan, err: err}
		})
	}}
}

func (m *model) acceptMachineCapturePlan(msg machineCapturePlanMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return
	}
	if msg.plan == nil {
		m.machine.err = fmt.Errorf("capture preview is empty")
		return
	}
	plan := *msg.plan
	m.confirm = &pages.Confirm{Prompt: fmt.Sprintf("Capture %s for %s at %s?", plan.Install.Skill, plan.Target, plan.Path), OnYes: func() {
		if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
			return
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			err := svc.ApplyMachineCapture(ctx, plan)
			return machineCaptureDoneMsg{sequence: seq, scope: msg.scope, err: err}
		}
	}}
}

func (m *model) acceptMachineCaptureDone(msg machineCaptureDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence || msg.scope != m.machineScope() {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	m.setStatus("installation captured")
	return m.startMachineScan()
}
