package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// FrameTop renders the top border of the nnn-style framed layout, with the
// title truncated (ANSI-aware) to fit.
func FrameTop(inner int, title string) string {
	t := ansi.Truncate(title, max(0, inner-2), "")
	pad := max(1, inner-1-lipgloss.Width(t))
	return "┌─" + t + strings.Repeat("─", pad) + "┐"
}

// FrameSep renders a horizontal separator matching the frame width.
func FrameSep(inner int) string {
	return "├" + strings.Repeat("─", inner) + "┤"
}

// FrameBottom renders the bottom border of the framed layout.
func FrameBottom(inner int) string {
	return "└" + strings.Repeat("─", inner) + "┘"
}

// FitCell truncates content to w cells (ANSI-aware) and pads it to exactly w
// cells before applying st, so a background style spans the full row.
func FitCell(content string, w int, st lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	t := ansi.Truncate(content, w, "")
	t = t + strings.Repeat(" ", max(0, w-lipgloss.Width(t)))
	return st.Render(t)
}

// SplitLines splits s into lines, dropping one trailing newline if present.
func SplitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// PadLines fills the slice with blank full-width rows up to n entries so the
// surrounding frame keeps its height.
func PadLines(lines []string, inner, n int) []string {
	for len(lines) < n {
		lines = append(lines, FitCell("", inner, lipgloss.NewStyle()))
	}
	return lines
}
