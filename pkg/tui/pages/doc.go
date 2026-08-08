// Package pages holds the skm TUI's independently-owned screens.
//
// Task center and target editor are fully self-contained here: state, key
// handling, actions, and rendering. List and detail are not — they share
// five actions (install/uninstall/update/archive/delete) and all
// cursor/selection state, so only their rendering lives here as pure
// functions over small DTOs; pkg/tui keeps their state and key handling.
// See specs/004-tui-ux-review's CLAUDE.md entry and the plan that split this
// package for the coupling analysis behind that asymmetry.
package pages
