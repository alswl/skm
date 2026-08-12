package tui

import (
	"fmt"
	"strconv"

	"github.com/alswl/skm/skm/pkg/common"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// entryAction is one selectable row in the actions menu (`x`): the key that
// also triggers it directly, its label, whether it applies to the current
// selection, and the handler to run when chosen.
type entryAction struct {
	Key     string
	Label   string
	Enabled bool
	Run     func()
}

// entryActions lists every per-entry action for the actions menu, sharing the
// same availability rules as the list/detail footers (listBindings /
// detailBindings) so a menu item is only offered when its own key would
// actually do something — never offered only to be rejected on selection.
// "detail" is omitted while the detail page is already open, since it's the
// page you're on.
func (m *model) entryActions() []entryAction {
	if m.cursor >= len(m.filtered) {
		return nil
	}
	e := m.filtered[m.cursor]
	install := e.Status == common.StatusActive && len(m.installs.forEntry(e.Path)) > 0
	update := m.svc.Updatable(e)
	normalize := e.Status == common.StatusNonStandard || e.Status == common.StatusActive
	claim := e.Kind == common.KindSkill && (e.Status == common.StatusError || e.Status == common.StatusNonStandard)
	fix := e.Status == common.StatusActive && len(m.fixableTargets(e)) > 0
	archiveLabel := "archive"
	if e.Status == common.StatusArchived {
		archiveLabel = "unarchive"
	}

	var out []entryAction
	if !m.showDetail {
		out = append(out, entryAction{Key: "enter", Label: "detail", Enabled: true, Run: m.openDetail})
	}
	out = append(out,
		entryAction{Key: "i", Label: "installs", Enabled: install, Run: m.installSelected},
		entryAction{Key: "p", Label: "update", Enabled: update, Run: m.updateSelected},
		entryAction{Key: "n", Label: "normalize", Enabled: normalize, Run: m.normalizeSelected},
		entryAction{Key: "c", Label: "claim → self-build", Enabled: claim, Run: m.claimAndRepairSelected},
		entryAction{Key: "F", Label: "fix conflicts/dangling", Enabled: fix, Run: m.fixSelected},
		entryAction{Key: "a", Label: archiveLabel, Enabled: true, Run: m.archiveSelected},
		entryAction{Key: "d", Label: "delete", Enabled: true, Run: m.deleteSelected},
	)
	return out
}

// globalActions lists the actions menu's list-scoped items: actions that
// don't depend on the selected entry, and already have their own keys
// (discover, import, batch update, targets, task queue). Offered only from
// the list, not from detail — none of these keys do anything while
// showDetail is true today (handleDetailKey has no cases for them), and
// import in particular renders its address prompt only inside listView's
// status line (statusContent), so enabling it from detail would swallow
// keystrokes into a prompt the user can't see.
func (m *model) globalActions() []entryAction {
	if m.showDetail {
		return nil
	}
	return []entryAction{
		{Key: "o", Label: "discover", Enabled: true, Run: m.discoverExternal},
		{Key: "m", Label: "import", Enabled: true, Run: func() {
			m.importing = true
			m.importAddr = ""
		}},
		{Key: "P", Label: "batch update", Enabled: true, Run: m.batchUpdate},
		{Key: "t", Label: "targets", Enabled: true, Run: func() {
			m.showTargets = true
			m.targetsCursor = 0
		}},
		{Key: "J", Label: "job queue", Enabled: true, Run: func() {
			m.showTasks = true
			m.tasksCursor = 0
		}},
	}
}

// openActionsMenu opens a single-select picker of every currently-available
// action for the highlighted entry, plus the list-scoped global actions (key
// "x"), so acting doesn't require memorizing a shortcut — only the compact
// footer's handful of picks and the full `?` help table existed before this
// (req: "列表页面更多动作选择"). It works even with an empty filtered list
// (e.g. a search with no matches): the global actions don't need a selected
// entry, only the entry-specific ones do.
func (m *model) openActionsMenu() {
	var available []entryAction
	if m.cursor < len(m.filtered) {
		for _, a := range m.entryActions() {
			if a.Enabled {
				available = append(available, a)
			}
		}
	}
	available = append(available, m.globalActions()...)
	if len(available) == 0 {
		m.setStatus("actions: nothing available")
		return
	}
	items := make([]pages.PickerItem, len(available))
	for i, a := range available {
		items[i] = pages.PickerItem{Label: fmt.Sprintf("[%s] %s", a.Key, a.Label), Value: strconv.Itoa(i)}
	}
	title := "actions"
	if m.cursor < len(m.filtered) {
		title = "actions · " + m.filtered[m.cursor].Name
	}
	m.picker = &pages.Picker{
		Title:  title,
		Hint:   "[j/k] move  [enter] run  [esc/q] cancel",
		Single: true,
		Items:  items,
		OnConfirm: func(sel []pages.PickerItem) {
			if len(sel) == 0 {
				return
			}
			idx, err := strconv.Atoi(sel[0].Value)
			if err != nil || idx < 0 || idx >= len(available) {
				return
			}
			available[idx].Run()
		},
	}
}
