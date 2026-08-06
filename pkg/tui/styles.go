package tui

import "github.com/charmbracelet/lipgloss"

// Theme — the single source of Lip Gloss styles (go-tui-guides.md: "One theme
// source… no inline lipgloss.NewStyle() scattered through View"). Colors use
// AdaptiveColor so they read on both light and dark terminals and degrade to
// plain text under NO_COLOR / non-TTY.
var (
	styleDim = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	// styleGroup is the dim, bold section header dividing the list by source.
	styleGroup  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "38"})
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "27", Dark: "39"})
	stylePrompt = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "27", Dark: "39"})
	// styleCursor highlights the current row (nnn-style solid cursor line).
	styleCursor = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "0"}).
			Background(lipgloss.AdaptiveColor{Light: "212", Dark: "141"})
	// styleStatusBar is the full-width bottom status bar.
	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"}).
			Background(lipgloss.AdaptiveColor{Light: "250", Dark: "236"})
)
