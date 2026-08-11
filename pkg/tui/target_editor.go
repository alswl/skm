package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/tui/components"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// targetWizard drives the plain-text steps of adding a target (name,
// platform, path) or editing an existing one's path. The accepts/strategy
// steps that follow are ordinary picker modals (m.picker), so this struct
// only needs to exist while a text field is being entered
// (002-open-provider-target FR-011, contracts/tui-target-editor.md).
type targetWizard struct {
	editing bool // true = editing an existing target's path only
	step    int  // add: 0=name,1=platform,2=path; edit: 0=path
	draft   common.InstallTarget
	text    string
}

const (
	wizardStepName = iota
	wizardStepPlatform
	wizardStepPath
)

// prompt returns the label for the current text step.
func (w *targetWizard) prompt() string {
	if w.editing {
		return "Path"
	}
	switch w.step {
	case wizardStepName:
		return "Name"
	case wizardStepPlatform:
		return "Platform"
	default:
		return "Path"
	}
}

// handleTargetsKey drives the target list (FR-011): navigate, add, edit
// (path), remove, or return to the main list. Edit/remove check the same
// selection condition their footer binding advertises (targetsBindings) and
// set a specific reason instead of a silent no-op when unavailable (FR-002).
func (m *model) handleTargetsKey(msg tea.KeyMsg) tea.Cmd {
	targets := m.svc.Cfg.Targets
	switch {
	case key.Matches(msg, m.keys.MoveDown):
		if m.targetsCursor < len(targets)-1 {
			m.targetsCursor++
		}
	case key.Matches(msg, m.keys.MoveUp):
		if m.targetsCursor > 0 {
			m.targetsCursor--
		}
	case key.Matches(msg, m.keys.TargetAdd):
		m.targetWizard = &targetWizard{step: wizardStepName}
	case key.Matches(msg, m.keys.Enter), key.Matches(msg, m.keys.Detail):
		if m.targetsCursor >= len(targets) {
			m.setStatus("edit path: no target selected")
			return nil
		}
		t := targets[m.targetsCursor]
		m.targetWizard = &targetWizard{editing: true, draft: t, text: t.Path}
	case key.Matches(msg, m.keys.Delete):
		if m.targetsCursor >= len(targets) {
			m.setStatus("remove target: no target selected")
			return nil
		}
		name := targets[m.targetsCursor].Name
		m.confirm = &pages.Confirm{
			Prompt: fmt.Sprintf("Remove target %q? Installed assets are left untouched.", name),
			OnYes:  func() { m.submitTargetRemove(name) },
		}
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Targets), key.Matches(msg, m.keys.Quit):
		m.showTargets = false
	}
	return nil
}

// targetsBindings is the target-editor footer's availability matrix,
// mirroring listBindings/detailBindings (FR-009): edit path and remove need a
// selected target.
func (m model) targetsBindings() []pages.HintItem {
	selected := m.targetsCursor < len(m.svc.Cfg.Targets)
	return []pages.HintItem{
		{Keys: "a", Label: "add", Enabled: true},
		{Keys: "enter", Label: "edit path", Enabled: selected},
		{Keys: "d", Label: "remove", Enabled: selected},
		{Keys: "j/k", Label: "move", Enabled: true},
		{Keys: "esc/t/q", Label: "back", Enabled: true},
	}
}

// targetsHint renders the target-editor footer, dimming bindings unavailable
// for the current selection (mirrors listHint/detailHint).
func (m model) targetsHint() string {
	var parts []string
	for _, b := range m.targetsBindings() {
		parts = append(parts, pages.HintBinding(b.Keys, b.Label, b.Enabled))
	}
	return strings.Join(parts, "  ")
}

// handleTargetWizardKey drives the active text-entry step.
func (m *model) handleTargetWizardKey(msg tea.KeyMsg) tea.Cmd {
	w := m.targetWizard
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.advanceTargetWizard()
	case key.Matches(msg, m.keys.Esc):
		m.targetWizard = nil
		m.setStatus("target: cancelled")
	case key.Matches(msg, m.keys.Backspace):
		if len(w.text) > 0 {
			w.text = w.text[:len(w.text)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			w.text += string(msg.Runes)
		}
	}
	return nil
}

// advanceTargetWizard commits the current text step and moves to the next
// one, opening the accepts picker (add) or submitting the update (edit) once
// the text steps are done.
func (m *model) advanceTargetWizard() {
	w := m.targetWizard
	value := strings.TrimSpace(w.text)
	if w.editing {
		name := w.draft.Name
		m.targetWizard = nil
		if value == "" {
			m.setStatus("target update: path must be non-empty")
			return
		}
		m.submitTargetPathUpdate(name, value)
		return
	}
	switch w.step {
	case wizardStepName:
		if value == "" {
			m.setStatus("target add: name must be non-empty")
			return
		}
		w.draft.Name = value
		w.text = ""
		w.step = wizardStepPlatform
	case wizardStepPlatform:
		w.draft.Platform = value
		w.text = ""
		w.step = wizardStepPath
	default: // wizardStepPath
		if value == "" {
			m.setStatus("target add: path must be non-empty")
			return
		}
		w.draft.Path = value
		draft := w.draft
		m.targetWizard = nil
		m.openAcceptsPicker(draft)
	}
}

// openAcceptsPicker asks which kinds the new target accepts, then chains into
// per-kind strategy selection (auto-resolved when only one strategy is
// kind-compatible).
func (m *model) openAcceptsPicker(draft common.InstallTarget) {
	m.picker = &pages.Picker{
		Title: "target accepts",
		Hint:  "[space] toggle  [enter] confirm  [esc/q] cancel",
		Items: []pages.PickerItem{
			{Label: "skill", Value: string(common.KindSkill)},
			{Label: "command", Value: string(common.KindCommand)},
		},
		OnConfirm: func(sel []pages.PickerItem) {
			if len(sel) == 0 {
				m.setStatus("target add: cancelled (no kinds selected)")
				return
			}
			kinds := make([]common.EntryKind, len(sel))
			for i, it := range sel {
				kinds[i] = common.EntryKind(it.Value)
			}
			draft.Accepts = kinds
			draft.Strategies = map[common.EntryKind]common.InstallStrategy{}
			m.resolveStrategy(draft, kinds, 0)
		},
	}
}

// kindStrategies lists the strategies compatible with kind: the built-in
// shapes (target-config.md) plus any loaded Target plugin whose declared
// capability covers kind (or that declares no capability at all, matching
// the same permissive fallback providers.Capability uses).
func (m *model) kindStrategies(kind common.EntryKind) []common.InstallStrategy {
	var out []common.InstallStrategy
	switch kind {
	case common.KindSkill:
		out = []common.InstallStrategy{common.StrategySkillSymlink}
	case common.KindCommand:
		out = []common.InstallStrategy{common.StrategyCommandSymlink, common.StrategyCommandMarker, common.StrategyCommandAdapter}
	}
	ids := make([]string, 0, len(m.svc.TargetPlugins))
	for id := range m.svc.TargetPlugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		cap := m.svc.TargetPlugins[id].Capability()
		if len(cap.Kinds) == 0 || slices.Contains(cap.Kinds, kind) {
			out = append(out, common.PluginStrategy(id))
		}
	}
	return out
}

// resolveStrategy assigns kinds[idx]'s strategy — automatically when only one
// is kind-compatible, otherwise via a single-select picker — then recurses,
// finally submitting the add once every kind is resolved.
func (m *model) resolveStrategy(draft common.InstallTarget, kinds []common.EntryKind, idx int) {
	if idx >= len(kinds) {
		m.submitTargetAdd(draft)
		return
	}
	kind := kinds[idx]
	options := m.kindStrategies(kind)
	if len(options) == 1 {
		draft.Strategies[kind] = options[0]
		m.resolveStrategy(draft, kinds, idx+1)
		return
	}
	items := make([]pages.PickerItem, len(options))
	for i, s := range options {
		items[i] = pages.PickerItem{Label: string(s), Value: string(s)}
	}
	m.picker = &pages.Picker{
		Title:  "strategy for " + string(kind),
		Hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		Single: true,
		Items:  items,
		OnConfirm: func(sel []pages.PickerItem) {
			if len(sel) == 0 {
				m.setStatus("target add: cancelled")
				return
			}
			draft.Strategies[kind] = common.InstallStrategy(sel[0].Value)
			m.resolveStrategy(draft, kinds, idx+1)
		},
	}
}

// submitTargetAdd, submitTargetPathUpdate, submitTargetRemove run the target
// mutation through the shared services layer in the background (FR-011,
// FR-025), keeping the TUI a thin adapter with no direct writes.

func (m *model) submitTargetAdd(draft common.InstallTarget) {
	m.submitJob("add target "+draft.Name, func(_ context.Context) (any, error) {
		if _, err := m.svc.TargetAdd(draft); err != nil {
			return nil, err
		}
		return fmt.Sprintf("added target %s", draft.Name), nil
	})
}

func (m *model) submitTargetPathUpdate(name, path string) {
	m.submitJob("update target "+name, func(_ context.Context) (any, error) {
		if _, err := m.svc.TargetUpdate(name, func(t *common.InstallTarget) { t.Path = path }); err != nil {
			return nil, err
		}
		return fmt.Sprintf("updated target %s", name), nil
	})
}

func (m *model) submitTargetRemove(name string) {
	m.submitJob("remove target "+name, func(_ context.Context) (any, error) {
		if err := m.svc.TargetRemove(name); err != nil {
			return nil, err
		}
		return fmt.Sprintf("removed target %s", name), nil
	})
}

// targetsView renders the target list as a full-screen framed page.
// Column widths for the targets table, following the entry list's rule: every
// column but the last truncates rather than grows, so one long name cannot
// push the columns after it out of line with the header and with the other
// rows. Path is last and takes whatever is left.
const (
	targetNameColWidth     = 16
	targetPlatformColWidth = 10
	targetAcceptsColWidth  = 16
)

// targetsHeaderRow labels the targets table, like installHeaderRow does for
// the entry list. Its caller prefixes rowGutter.
func targetsHeaderRow() string {
	return components.StyleDim.Render(
		truncPad("name", targetNameColWidth) + " " +
			truncPad("platform", targetPlatformColWidth) + " " +
			truncPad("accepts", targetAcceptsColWidth) + " path")
}

func targetRow(name, platform, accepts, path string) string {
	return truncPad(name, targetNameColWidth) + " " +
		truncPad(platform, targetPlatformColWidth) + " " +
		truncPad(accepts, targetAcceptsColWidth) + " " + path
}

func (m model) targetsView() string {
	targets := m.svc.Cfg.Targets
	inner := maxInt(20, m.width) - 2
	var body strings.Builder
	body.WriteString(components.FitCell(rowGutter+targetsHeaderRow(), inner, lipgloss.NewStyle()) + "\n")
	if len(targets) == 0 {
		body.WriteString(components.StyleDim.Render("no targets configured"))
	}
	for i, t := range targets {
		accepts := make([]string, len(t.Accepts))
		for j, k := range t.Accepts {
			accepts[j] = string(k)
		}
		row := targetRow(t.Name, t.Platform, strings.Join(accepts, ","), t.Path)
		if i == m.targetsCursor {
			body.WriteString(components.FitCell(cursorGutter+row, inner, components.StyleCursor) + "\n")
		} else {
			body.WriteString(components.FitCell(rowGutter+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	for _, inv := range m.svc.Cfg.InvalidTargets {
		body.WriteString(components.FitCell(rowGutter+components.StyleDim.Render("invalid: "+inv.Reason), inner, lipgloss.NewStyle()) + "\n")
	}
	return m.framedPage(" skm · targets ", strings.TrimRight(body.String(), "\n"), m.targetsHint())
}

// targetWizardView renders the active text-entry step as a full-screen
// framed page, matching the picker/confirm/tasks views.
func (m model) targetWizardView() string {
	body := components.StylePrompt.Render(m.targetWizard.prompt()+": ") + m.targetWizard.text + "▏"
	return m.framedPage(" skm · target "+addOrEdit(m.targetWizard.editing), body, "[enter] next  [esc] cancel")
}

func addOrEdit(editing bool) string {
	if editing {
		return "edit"
	}
	return "add"
}
