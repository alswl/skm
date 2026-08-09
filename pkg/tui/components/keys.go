package components

import "github.com/charmbracelet/bubbles/key"

// KeyMap is the single set of TUI key bindings (tui-contract.md). Bindings
// live in one place so key matching, the help bar, and documentation stay in
// sync (go-tui-guides.md: "Model keys with bubbles/key… no bare string
// comparisons").
type KeyMap struct {
	MoveUp   key.Binding
	MoveDown key.Binding
	PagePrev key.Binding
	PageNext key.Binding
	First    key.Binding
	Last     key.Binding

	Search       key.Binding
	ClearSearch  key.Binding
	ShowArchived key.Binding
	TabNext      key.Binding
	TabPrev      key.Binding

	Install     key.Binding
	Update      key.Binding
	BatchUpdate key.Binding
	Archive     key.Binding
	Delete      key.Binding
	Discover    key.Binding
	Import      key.Binding
	Targets     key.Binding
	TargetAdd   key.Binding
	Normalize   key.Binding
	Fix         key.Binding
	ActionsMenu key.Binding
	Refresh     key.Binding

	Queue  key.Binding
	Cancel key.Binding
	Detail key.Binding
	Help   key.Binding
	Quit   key.Binding

	Enter     key.Binding
	Esc       key.Binding
	Backspace key.Binding

	// Modal / task-center bindings (req-2 §1).
	Toggle    key.Binding
	Yes       key.Binding
	No        key.Binding
	CancelSel key.Binding
	CancelAll key.Binding
	ClearDone key.Binding
}

// DefaultKeys returns the full key set. Every binding keeps the exact key from
// tui-contract.md; `?` is added as the help toggle (Phase 13).
func DefaultKeys() KeyMap {
	return KeyMap{
		MoveUp:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
		MoveDown:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
		PagePrev:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h", "prev page")),
		PageNext:     key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l", "next page")),
		First:        key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "first")),
		Last:         key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "last")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		ClearSearch:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear search")),
		ShowArchived: key.NewBinding(key.WithKeys("."), key.WithHelp(".", "toggle archived")),
		TabNext:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next provider tab")),
		TabPrev:      key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev provider tab")),
		Install:      key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "installs")),
		Update:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "update")),
		BatchUpdate:  key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "batch update")),
		Archive:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive")),
		Delete:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Discover:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "discover")),
		Import:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "import")),
		Targets:      key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "targets")),
		TargetAdd:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add target")),
		Normalize:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "normalize")),
		Fix:          key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "fix conflicts/dangling")),
		ActionsMenu:  key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "actions menu")),
		Refresh:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Queue:        key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "job queue")),
		Cancel:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel task")),
		Detail:       key.NewBinding(key.WithKeys("enter", "v"), key.WithHelp("enter", "detail")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Enter:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Esc:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Backspace:    key.NewBinding(key.WithKeys("backspace"), key.WithHelp("backspace", "delete")),
		Toggle:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		Yes:          key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:           key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
		CancelSel:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel job")),
		CancelAll:    key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "cancel all")),
		ClearDone:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "clear done")),
	}
}

// FullHelp returns the grouped full-key table shown on `?`.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.MoveDown, k.MoveUp, k.PagePrev, k.PageNext, k.First, k.Last, k.Search, k.ClearSearch, k.ShowArchived, k.TabNext, k.TabPrev},
		{k.Detail, k.Install, k.Update, k.BatchUpdate, k.Archive, k.Delete, k.Discover, k.Import, k.Targets, k.Normalize, k.Fix, k.ActionsMenu, k.Refresh},
		{k.Queue, k.Cancel, k.Help, k.Quit},
	}
}
