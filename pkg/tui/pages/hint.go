package pages

import "github.com/alswl/skm/skm/pkg/tui/components"

// HintItem is one footer binding and whether it applies to the current
// selection/context.
type HintItem struct {
	Keys    string
	Label   string
	Enabled bool
}

// HintBinding renders a key binding, dimmed when the action is not available
// for the current selection.
func HintBinding(keys, label string, enabled bool) string {
	if !enabled {
		return components.StyleDim.Render("[" + keys + "] " + label)
	}
	return "[" + keys + "] " + label
}
