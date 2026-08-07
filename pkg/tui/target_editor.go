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
// (path), remove, or return to the main list.
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
		if m.targetsCursor < len(targets) {
			t := targets[m.targetsCursor]
			m.targetWizard = &targetWizard{editing: true, draft: t, text: t.Path}
		}
	case key.Matches(msg, m.keys.Delete):
		if m.targetsCursor < len(targets) {
			name := targets[m.targetsCursor].Name
			m.confirm = &confirm{
				prompt: fmt.Sprintf("Remove target %q? Installed assets are left untouched.", name),
				onYes:  func() { m.submitTargetRemove(name) },
			}
		}
	case key.Matches(msg, m.keys.Esc), key.Matches(msg, m.keys.Targets), key.Matches(msg, m.keys.Quit):
		m.showTargets = false
	}
	return nil
}

// handleTargetWizardKey drives the active text-entry step.
func (m *model) handleTargetWizardKey(msg tea.KeyMsg) tea.Cmd {
	w := m.targetWizard
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.advanceTargetWizard()
	case key.Matches(msg, m.keys.Esc):
		m.targetWizard = nil
		m.status = "target: cancelled"
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
			m.status = "target update: path must be non-empty"
			return
		}
		m.submitTargetPathUpdate(name, value)
		return
	}
	switch w.step {
	case wizardStepName:
		if value == "" {
			m.status = "target add: name must be non-empty"
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
			m.status = "target add: path must be non-empty"
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
	m.picker = &picker{
		title: "target accepts",
		hint:  "[space] toggle  [enter] confirm  [esc/q] cancel",
		items: []pickerItem{
			{label: "skill", value: string(common.KindSkill)},
			{label: "command", value: string(common.KindCommand)},
		},
		onConfirm: func(sel []pickerItem) {
			if len(sel) == 0 {
				m.status = "target add: cancelled (no kinds selected)"
				return
			}
			kinds := make([]common.EntryKind, len(sel))
			for i, it := range sel {
				kinds[i] = common.EntryKind(it.value)
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
		out = []common.InstallStrategy{common.StrategyCommandMarker, common.StrategyCommandAdapter}
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
	items := make([]pickerItem, len(options))
	for i, s := range options {
		items[i] = pickerItem{label: string(s), value: string(s)}
	}
	m.picker = &picker{
		title:  "strategy for " + string(kind),
		hint:   "[j/k] move  [enter] choose  [esc/q] cancel",
		single: true,
		items:  items,
		onConfirm: func(sel []pickerItem) {
			if len(sel) == 0 {
				m.status = "target add: cancelled"
				return
			}
			draft.Strategies[kind] = common.InstallStrategy(sel[0].value)
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
func (m model) targetsView() string {
	targets := m.svc.Cfg.Targets
	inner := maxInt(20, m.width) - 2
	var body strings.Builder
	if len(targets) == 0 {
		body.WriteString(styleDim.Render("no targets configured"))
	}
	for i, t := range targets {
		accepts := make([]string, len(t.Accepts))
		for j, k := range t.Accepts {
			accepts[j] = string(k)
		}
		row := fmt.Sprintf("%-16s %-10s %-16s %s", t.Name, t.Platform, strings.Join(accepts, ","), t.Path)
		if i == m.targetsCursor {
			body.WriteString(fitCell("  ▶ "+row, inner, styleCursor) + "\n")
		} else {
			body.WriteString(fitCell("    "+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	for _, inv := range m.svc.Cfg.InvalidTargets {
		body.WriteString(fitCell("    "+styleDim.Render("invalid: "+inv.Reason), inner, lipgloss.NewStyle()) + "\n")
	}
	hint := "[a] add  [enter] edit path  [d] remove  [j/k] move  [esc/t/q] back"
	return m.framedPage(" skm · targets ", strings.TrimRight(body.String(), "\n"), hint)
}

// targetWizardView renders the active text-entry step as a full-screen
// framed page, matching the picker/confirm/tasks views.
func (m model) targetWizardView() string {
	body := stylePrompt.Render(m.targetWizard.prompt()+": ") + m.targetWizard.text + "▏"
	return m.framedPage(" skm · target "+addOrEdit(m.targetWizard.editing), body, "[enter] next  [esc] cancel")
}

func addOrEdit(editing bool) string {
	if editing {
		return "edit"
	}
	return "add"
}
