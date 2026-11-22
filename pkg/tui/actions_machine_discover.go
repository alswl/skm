package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type machineDiscoverMsg struct {
	sequence    uint64
	paths       []string
	diagnostics []string
	err         error
}

type machineRegisterDoneMsg struct {
	sequence uint64
	scope    string
	err      error
}

type machinePathDoneMsg struct {
	sequence  uint64
	directory string
	path      string
	err       error
}

func (m *model) beginMachinePath(directory string, onReturn func()) {
	m.machine.pathEditing, m.machine.pathInput = "path", ""
	m.machine.pathDirectory, m.machine.pathReturn = directory, onReturn
}

func (m *model) acceptMachinePathDone(msg machinePathDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.setStatus(msg.err.Error())
		m.machine.pathEditing, m.machine.pathInput = "path", msg.path
		return nil
	}
	for i := range m.machine.allDirectories {
		if m.machine.allDirectories[i].ID == msg.directory {
			m.machine.allDirectories[i].Path = msg.path
		}
	}
	for i := range m.machine.directories {
		if m.machine.directories[i].ID == msg.directory {
			m.machine.directories[i].Path = msg.path
		}
	}
	m.setStatus("local path saved")
	if resume := m.machine.pathReturn; resume != nil {
		m.machine.pathReturn = nil
		resume()
		return nil
	}
	return m.startMachineScan()
}

func (m *model) handleMachinePathInput(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Esc):
		if m.machine.pathEditing == "discover" && m.machine.installRegistration != nil {
			m.machine.pathEditing = ""
			m.cancelInstallRegistration()
			return nil
		}
		m.machine.pathEditing = ""
		if resume := m.machine.pathReturn; resume != nil {
			m.machine.pathReturn = nil
			resume()
		}
	case key.Matches(msg, m.keys.Backspace):
		runes := []rune(m.machine.pathInput)
		if len(runes) > 0 {
			m.machine.pathInput = string(runes[:len(runes)-1])
		}
	case key.Matches(msg, m.keys.Enter):
		path, action := strings.TrimSpace(m.machine.pathInput), m.machine.pathEditing
		if path == "" {
			m.setStatus("path is empty")
			return nil
		}
		m.machine.pathEditing = ""
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		if action == "discover" {
			return func() tea.Msg {
				report, err := svc.DiscoverDirectoriesReport(ctx, path)
				var paths, diagnostics []string
				for _, item := range report {
					if item.Error != "" {
						diagnostics = append(diagnostics, item.Path+": "+item.Error)
					} else if !item.Registered {
						paths = append(paths, item.Path)
					}
				}
				return machineDiscoverMsg{sequence: seq, paths: paths, diagnostics: diagnostics, err: err}
			}
		}
		scope := m.machine.pathDirectory
		return func() tea.Msg {
			err := svc.SetDirectoryPath(ctx, scope, path)
			return machinePathDoneMsg{sequence: seq, directory: scope, path: path, err: err}
		}
	case msg.Type == tea.KeyRunes:
		m.machine.pathInput += string(msg.Runes)
	case msg.Type == tea.KeySpace:
		m.machine.pathInput += " "
	}
	return nil
}

func (m *model) cancelInstallRegistration() {
	if resume := m.machine.installRegistration; resume != nil {
		m.machine.installRegistration = nil
		resume()
	}
}

func (m *model) acceptMachineDiscover(msg machineDiscoverMsg) {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		m.cancelInstallRegistration()
		return
	}
	if len(msg.diagnostics) != 0 {
		m.machine.result = "Discovery diagnostics:\n" + strings.Join(msg.diagnostics, "\n")
	}
	registered := map[string]bool{}
	for _, dir := range m.machine.allDirectories {
		registered[dir.Path] = true
	}
	items := []pages.PickerItem{}
	for _, path := range msg.paths {
		if !registered[path] {
			items = append(items, pages.PickerItem{Label: fmt.Sprintf("%s  (%s)", filepath.Base(path), filepath.Dir(path)), Value: path})
		}
	}
	if len(items) == 0 {
		m.cancelInstallRegistration()
		m.setStatus("no new Git directories found")
		return
	}
	m.picker = &pages.Picker{Title: "register directories", Hint: "space select  enter register  esc cancel", Items: items, OnCancel: m.cancelInstallRegistration, OnConfirm: func(selected []pages.PickerItem) {
		if len(selected) == 0 || !m.machine.open {
			m.cancelInstallRegistration()
			return
		}
		used := map[string]bool{}
		for _, dir := range m.machine.allDirectories {
			used[dir.ID] = true
		}
		entries := map[string]string{}
		firstID := ""
		for _, item := range selected {
			base := machineDirectoryID(filepath.Base(item.Value))
			id := base
			for suffix := 2; used[id]; suffix++ {
				ending := fmt.Sprintf("-%d", suffix)
				prefix := base
				if len(prefix)+len(ending) > 63 {
					prefix = strings.TrimRight(prefix[:63-len(ending)], "-")
				}
				id = prefix + ending
			}
			if firstID == "" {
				firstID = id
			}
			used[id] = true
			entries[id] = item.Value
		}
		m.machine.sequence++
		m.machine.scanning = true
		seq, svc, ctx := m.machine.sequence, m.svc, m.ctx
		m.machinePending = func() tea.Msg {
			err := svc.RegisterDirectories(ctx, entries, nil)
			return machineRegisterDoneMsg{sequence: seq, scope: firstID, err: err}
		}
	}}
}

func machineDirectoryID(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
		if b.Len() >= 63 {
			break
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "directory"
	}
	return id
}

func (m *model) acceptMachineRegisterDone(msg machineRegisterDoneMsg) tea.Cmd {
	if !m.machine.open || msg.sequence != m.machine.sequence {
		return nil
	}
	m.machine.scanning = false
	if msg.err != nil {
		m.machine.err = msg.err
		return nil
	}
	m.setStatus("directories updated")
	m.openMachineDirectories()
	m.machine.requestedScope = msg.scope
	return nil
}
