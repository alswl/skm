package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/alswl/skm/skm/pkg/common"
)

// Theme — the single source of Lip Gloss styles (go-tui-guides.md: "One theme
// source… no inline lipgloss.NewStyle() scattered through View"). Colors use
// AdaptiveColor so they read on both light and dark terminals and degrade to
// plain text under NO_COLOR / non-TTY.
var (
	StyleDim = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	// StyleGroup is the dim, bold section header dividing the list by source.
	StyleGroup  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "24", Dark: "38"})
	StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "27", Dark: "39"})
	StylePrompt = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "27", Dark: "39"})
	// StyleCursor highlights the current row. nnn's own cursor line isn't a
	// fixed color — it reverses whatever fg/bg the terminal already has, so
	// it never clashes with the user's theme; a hardcoded magenta/purple
	// background did, and is too high-contrast against most themes.
	StyleCursor = lipgloss.NewStyle().Reverse(true)
	// StyleStatusBar is the full-width bottom status bar.
	StyleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"}).
			Background(lipgloss.AdaptiveColor{Light: "250", Dark: "236"})
)

// nnn-inspired zone colors for the flat list: kind colors distinguish skill vs
// command (blue like nnn dirs, green like nnn executables), and status colors
// flag non-standard (yellow), archived (dim), error (red). Active is the
// default foreground — the normal state needs no flag.
var (
	styleKindSkill         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "27", Dark: "75"})
	styleKindCommand       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "71"})
	styleStatusNonStandard = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "178"})
	styleStatusArchived    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	styleStatusError       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "203"})
	styleInstallInstalled  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "71"})
	styleInstallConflict   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "203"})
	styleInstallDangling   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "178"})
)

// InstallNA marks a target column that structurally cannot receive an
// entry's kind (the target's Accepts doesn't list it) — distinct from
// InstallAbsent, which means the target could receive the entry but doesn't
// have it yet (FR-041 per-target columns).
const InstallNA common.InstallState = "na"

// StyleForKind colors the kind cell (blue for skills like nnn dirs, green for
// commands like nnn executables).
func StyleForKind(k common.EntryKind) lipgloss.Style {
	if k == common.KindCommand {
		return styleKindCommand
	}
	return styleKindSkill
}

// InstallIcon renders one target cell's glyph and color (FR-041 per-target
// columns): a check when installed, a cross for a conflict, a warning sign
// for a dangling link, and an empty cell for both absent installs and targets
// whose kind doesn't match. Keeping every non-problem, non-installed cell
// blank makes the per-target columns easy to scan.
func InstallIcon(s common.InstallState) (string, lipgloss.Style) {
	switch s {
	case common.InstallInstalled:
		return "✓", styleInstallInstalled
	case common.InstallConflict:
		return "✗", styleInstallConflict
	case common.InstallDangling:
		return "⚠", styleInstallDangling
	default:
		return "", lipgloss.NewStyle()
	}
}

// StyleForStatus colors the status cell: yellow for non-standard (needs
// attention), dim for archived, red for error; active is the default.
func StyleForStatus(s common.Status) lipgloss.Style {
	switch s {
	case common.StatusNonStandard:
		return styleStatusNonStandard
	case common.StatusArchived:
		return styleStatusArchived
	case common.StatusError:
		return styleStatusError
	default:
		return lipgloss.NewStyle()
	}
}
