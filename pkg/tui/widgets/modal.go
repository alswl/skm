package widgets

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/tui/components"
)

// PickerItem is one selectable row in a Picker modal.
type PickerItem struct {
	Label   string
	Value   string
	Checked bool
}

// Picker is a reusable modal: a multi-select checkbox list (targets,
// discovered skills) or a single-select radio list (provider, kind).
// Selection is confirmed with Enter and cancelled with Esc; OnDelete, when
// set, wires a secondary destructive action (discover delete) to the Delete
// key. (FR-036/037/038)
type Picker struct {
	Title     string
	Hint      string
	Items     []PickerItem
	Cursor    int
	Single    bool
	OnConfirm func(sel []PickerItem)
	OnDelete  func(sel []PickerItem)
}

// MoveCursor moves the highlighted row by delta, clamped to the item range.
func (p *Picker) MoveCursor(delta int) {
	p.Cursor += delta
	if p.Cursor < 0 {
		p.Cursor = 0
	}
	if p.Cursor > len(p.Items)-1 {
		p.Cursor = len(p.Items) - 1
	}
}

// ToggleCurrent flips the highlighted row's checked state (multi-select only).
func (p *Picker) ToggleCurrent() {
	if p.Single || len(p.Items) == 0 {
		return
	}
	p.Items[p.Cursor].Checked = !p.Items[p.Cursor].Checked
}

// Selection returns the confirmed items: the highlighted one for a radio
// picker, otherwise every checked item.
func (p *Picker) Selection() []PickerItem {
	if p.Single {
		if len(p.Items) == 0 {
			return nil
		}
		return []PickerItem{p.Items[p.Cursor]}
	}
	var out []PickerItem
	for _, it := range p.Items {
		if it.Checked {
			out = append(out, it)
		}
	}
	return out
}

// Confirm is a yes/no modal guarding a destructive action (FR-040).
type Confirm struct {
	Prompt string
	OnYes  func()
}

// PickerView renders the active picker as a full-screen framed page.
func PickerView(width, height int, status string, p *Picker) string {
	inner := maxInt(20, width) - 2
	var body strings.Builder
	for i, it := range p.Items {
		mark := "[ ]"
		switch {
		case p.Single && it.Checked:
			mark = "(•)"
		case p.Single:
			mark = "( )"
		case it.Checked:
			mark = "[x]"
		}
		row := mark + " " + it.Label
		if i == p.Cursor {
			body.WriteString(components.FitCell("  ▶ "+row, inner, components.StyleCursor) + "\n")
		} else {
			body.WriteString(components.FitCell("    "+row, inner, lipgloss.NewStyle()) + "\n")
		}
	}
	return FramedPage(width, height, " skm · "+p.Title+" ", strings.TrimRight(body.String(), "\n"), status, p.Hint)
}

// ConfirmView renders the active confirmation as a full-screen framed page.
func ConfirmView(width, height int, status string, c *Confirm) string {
	body := components.StylePrompt.Render(c.Prompt)
	return FramedPage(width, height, " skm · confirm ", body, status, "[y] yes   [n/esc/q] no")
}

// FramedPage renders title/body/hint inside the box-drawing frame, matching
// the list and detail pages so modals read as full-screen nnn-style views. It
// also renders status (the same last-action/job-failure line list and detail
// show) so a background job that finishes while a modal is open — tasks,
// targets, target wizard, picker, or confirm — is never invisible (FR-003,
// FR-004, contract §2).
func FramedPage(width, height int, title, body, status, hint string) string {
	w := maxInt(20, width)
	h := maxInt(10, height)
	inner := w - 2
	rows := maxInt(1, h-6) // top, [body], sep, status, sep, hint, bottom
	var sb strings.Builder
	sb.WriteString(components.FrameTop(inner, title) + "\n")
	for _, l := range components.PadLines(components.SplitLines(body), inner, rows) {
		sb.WriteString("│" + components.FitCell(l, inner, lipgloss.NewStyle()) + "│\n")
	}
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(status, inner, components.StyleStatusBar) + "│\n")
	sb.WriteString(components.FrameSep(inner) + "\n")
	sb.WriteString("│" + components.FitCell(hint, inner, components.StyleStatusBar) + "│\n")
	sb.WriteString(components.FrameBottom(inner))
	return sb.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
