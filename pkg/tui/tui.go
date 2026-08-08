package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	isatty "github.com/mattn/go-isatty"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/components"
	"github.com/alswl/skm/skm/pkg/tui/pages"
	"github.com/alswl/skm/skm/pkg/utils/pagination"
)

// Run starts the TUI. It must only be called when the tool was launched with
// no known subcommand (FR-001). The UI never writes files directly; all
// mutations go through the shared services layer. A cancelled context stops
// the program (go-tui-guides.md: tea.WithContext).
func Run(ctx context.Context, svc *services.Services) error {
	if !isTerminal() {
		return fmt.Errorf("tui: requires an interactive terminal; use a subcommand (e.g. skm list --json) for scriptable output")
	}
	p := tea.NewProgram(initialModel(ctx, svc), tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// isTerminal reports whether both stdin and stdout are character devices, i.e.
// the program is not piped or running under CI (go-tui-guides.md: degrade
// gracefully when not a TTY).
func isTerminal() bool {
	return isTerminalFile(os.Stdin) && isTerminalFile(os.Stdout)
}

// isTerminalFile reports whether f is an interactive terminal (ioctl check;
// ModeCharDevice alone would accept /dev/null).
func isTerminalFile(f *os.File) bool {
	return isatty.IsTerminal(f.Fd())
}

// model is the top-level Bubble Tea model: a paged list on the left and a
// detail pane on the right (tui-contract.md).
type model struct {
	ctx          context.Context
	svc          *services.Services
	queue        *jobs.Queue
	entries      []*common.Entry // full scan result
	filtered     []*common.Entry // after search filter, sorted by source
	rows         []dispRow       // display rows: section headers interleaved with entries
	entryRow     []int           // entryRow[i] = index into rows for filtered[i]
	cursor       int             // index into filtered (selectable entries only)
	page         int             // 0-based page over display rows
	pageSize     int
	search       string
	searching    bool
	showArchived bool // archived entries are hidden until toggled with `.`
	importing    bool
	importAddr   string
	status       string // last action status line
	width        int
	height       int

	keys     components.KeyMap
	help     help.Model
	showHelp bool

	loading bool // true while the initial scan is still running (spinner shown in place of the list)
	spinner spinner.Model

	showDetail      bool   // true renders the full-screen detail page (Enter/v)
	detail          string // detail page content, built lazily in openDetail
	detailOffset    int    // scroll offset (lines) into the detail content; j/k scroll it
	detailTargets   int    // number of kind-matching targets (cached in Update; View must stay pure)
	detailInstalled bool   // whether the entry is installed in any matching target (cached in Update)

	// req-2 §1 modals and task center.
	picker       *pages.Picker            // active multi/single-select modal (nil when closed)
	confirm      *pages.Confirm           // active confirmation modal (nil when closed)
	showTasks    bool                     // task-center view (J)
	tasksCursor  int                      // cursor into the flattened task list
	installCol   map[string][]installCell // entry name -> one cell per configured target, in Cfg.Targets order (FR-041)
	forceRetries map[int64]forceRetry     // job id -> force-retry offer, for jobs that failed needing --force

	// 002-open-provider-target US2: Target configuration editor.
	showTargets   bool          // target list/editor view (t)
	targetsCursor int           // cursor into svc.Cfg.Targets
	targetWizard  *targetWizard // active add/edit text-entry step (nil when idle)

	// Provider filter tabs (Tab/Shift+Tab): providerTabs[0] is always
	// tabAll; providerTabs[providerTabIdx] is the active filter.
	providerTabs   []string
	providerTabIdx int

	// providerIcons maps a provider id (entry.ProviderID) to its declared
	// one-glyph icon, computed once at startup (the registry never changes
	// after Services.New()). Precomputed here rather than looked up in View,
	// since a plugin provider's Capability() makes a subprocess call and View
	// must stay pure/fast.
	providerIcons map[string]string
}

// tabAll and tabNone are the two synthetic provider-tab values: tabAll shows
// every entry (no filter); tabNone groups entries with no mode_id. Real
// provider mode_id values are never empty, so tabAll's "" never collides
// with a real tab.
const (
	tabAll  = ""
	tabNone = "none"
)

// initialModel returns a pointer model: the TUI runs on a single heap-allocated
// *model so every closure capturing the receiver (picker/confirm callbacks,
// job runners) always refers to the live model object. A value-based model
// would copy the receiver each Update and closures would mutate stale copies —
// the modals they open (confirm after a picker, next picker, status lines)
// would silently never appear in the running program.
func initialModel(ctx context.Context, svc *services.Services) *model {
	m := &model{
		ctx:           ctx,
		svc:           svc,
		queue:         jobs.New(32),
		pageSize:      20,
		keys:          components.DefaultKeys(),
		help:          help.New(),
		loading:       true,
		spinner:       spinner.New(),
		providerIcons: svc.ProviderIcons(),
	}
	return m
}

// unknownProviderIcon marks an entry whose ProviderID (including "", i.e. none
// recorded) has no registered/loaded provider declaring an icon — e.g. the
// "self-build" bucket (services.NewSelfBuild) covers ProviderID
// "self-build"; a truly empty or unrecognized ProviderID falls back here.
const unknownProviderIcon = "❓"

// providerIcon returns the icon declared by the provider that imported an
// entry (its ProviderID), or unknownProviderIcon when the ProviderID doesn't resolve
// to a provider that declared one — a pure map lookup, safe to call from
// View.
func (m model) providerIcon(modeID string) string {
	if icon, ok := m.providerIcons[modeID]; ok {
		return icon
	}
	return unknownProviderIcon
}

// applyEntries installs a fresh scan result and recomputes everything derived
// from it (install columns, provider tabs, filtered/display rows). Shared by
// the initial async scan (scanDoneMsg) and post-job rescans (handleJobDone).
func (m *model) applyEntries(entries []*common.Entry) {
	m.entries = entries
	m.computeInstallCols()
	m.computeProviderTabs()
	m.refreshFiltered()
}

// scanDoneMsg carries the result of the initial background scan.
type scanDoneMsg struct{ entries []*common.Entry }

// scanCmd runs the potentially slow filesystem scan off the Bubble Tea event
// loop so the program can render the loading spinner immediately instead of
// blocking before the first frame.
func scanCmd(svc *services.Services) tea.Cmd {
	return func() tea.Msg { return scanDoneMsg{entries: svc.Scan()} }
}

// computeProviderTabs derives the provider filter tabs from the current scan:
// tabAll, then every distinct mode_id in sorted order, then tabNone if any
// entry has no mode_id. Called after every scan (startup and job
// completion), like computeInstallCols.
func (m *model) computeProviderTabs() {
	seen := map[string]bool{}
	hasNone := false
	for _, e := range m.entries {
		if mid := e.ProviderIDValue(); mid != "" {
			seen[mid] = true
		} else {
			hasNone = true
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	tabs := append([]string{tabAll}, names...)
	if hasNone {
		tabs = append(tabs, tabNone)
	}
	m.providerTabs = tabs
	if m.providerTabIdx >= len(tabs) {
		m.providerTabIdx = 0 // the active tab vanished (e.g. its entries were archived/deleted): fall back to All
	}
}

// cycleProviderTab moves the active provider filter tab by delta (wrapping)
// and re-filters the list.
func (m *model) cycleProviderTab(delta int) {
	if len(m.providerTabs) == 0 {
		return
	}
	n := len(m.providerTabs)
	m.providerTabIdx = ((m.providerTabIdx+delta)%n + n) % n
	m.refreshFiltered()
}

// isDigitKey reports whether msg is a single '0'-'9' rune, used for direct
// provider-tab jumps (0=All, 1..=providers/none, in tab-bar order) alongside
// Tab/Shift+Tab cycling.
func isDigitKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] >= '0' && msg.Runes[0] <= '9'
}

// jumpToProviderTab switches directly to provider tab index n. Out-of-range
// digits (more tabs than 0-9, or fewer tabs than the pressed digit) are
// silently ignored rather than wrapping or erroring.
func (m *model) jumpToProviderTab(n int) {
	if n < 0 || n >= len(m.providerTabs) {
		return
	}
	m.providerTabIdx = n
	m.refreshFiltered()
}

// activeProviderTab returns the currently selected tab value (tabAll,
// tabNone, or a real mode_id).
func (m *model) activeProviderTab() string {
	if m.providerTabIdx <= 0 || m.providerTabIdx >= len(m.providerTabs) {
		return tabAll
	}
	return m.providerTabs[m.providerTabIdx]
}

// computeInstallCols derives each entry's per-target install-state cells for
// the list columns (FR-041): one cell per configured target, in Cfg.Targets
// order, so the list shows every target individually instead of a single
// collapsed summary. The filesystem reads happen here, off the View path; it
// is called after every scan (startup and job completion), not on filter or
// resize.
func (m *model) computeInstallCols() {
	targets := m.svc.Cfg.Targets
	m.installCol = make(map[string][]installCell, len(m.entries))
	for _, e := range m.entries {
		// Evaluated for every entry (not just active ones) so a non-standard or
		// moved entry's dangling/conflicting installs are still visible in the
		// list ("安装状态无法在 list 页面看到").
		cells := make([]installCell, len(targets))
		for i, t := range targets {
			state := components.InstallNA
			if t.AcceptsKind(e.Kind) {
				state = m.svc.Installer.State(e, t)
			}
			cells[i] = installCell{name: t.Name, state: state}
		}
		m.installCol[e.Name] = cells
	}
}

// refreshFiltered re-applies the provider tab and search filters and
// re-validates cursor/page (FR-009: selection and page stay valid across
// filtering/paging/resize).
func (m *model) refreshFiltered() {
	q := m.search
	tab := m.activeProviderTab()
	// A fresh slice so buildRows' sort never reorders the underlying scan slice.
	m.filtered = nil
	for _, e := range m.entries {
		if !m.showArchived && e.Status == common.StatusArchived {
			continue
		}
		if tab != tabAll {
			mid := e.ProviderIDValue()
			if tab == tabNone {
				if mid != "" {
					continue
				}
			} else if mid != tab {
				continue
			}
		}
		if q != "" && !containsFold(e.Name, q) && !containsFold(e.Description, q) {
			continue
		}
		m.filtered = append(m.filtered, e)
	}
	m.buildRows()
	m.clampView()
}

// dispRow is one rendered list line: a section header (mode_id / group) when
// header is non-empty, otherwise an entry referenced by entryIdx into filtered.
type dispRow struct {
	header   string
	entryIdx int
}

// buildRows sorts the filtered entries by source (mode_id, then group, then
// name) and interleaves a section header wherever that source changes, so the
// single-column list is visually divided by group (tui-contract.md). It records
// entryRow so navigation can map a cursor (entry index) onto a display row.
func (m *model) buildRows() {
	sort.SliceStable(m.filtered, func(i, j int) bool {
		a, b := m.filtered[i], m.filtered[j]
		if a.ProviderIDValue() != b.ProviderIDValue() {
			return a.ProviderIDValue() < b.ProviderIDValue()
		}
		if a.GroupValue() != b.GroupValue() {
			return a.GroupValue() < b.GroupValue()
		}
		return a.Name < b.Name
	})
	m.rows = m.rows[:0]
	if cap(m.entryRow) < len(m.filtered) {
		m.entryRow = make([]int, len(m.filtered))
	} else {
		m.entryRow = m.entryRow[:len(m.filtered)]
	}
	// The list is flat — no per-source section headers. Provider/group context
	// comes from the tabs, the cursor's status line, and the detail page, so
	// grouping headers in the list would just re-filter the page.
	for i := range m.filtered {
		m.entryRow[i] = len(m.rows)
		m.rows = append(m.rows, dispRow{entryIdx: i})
	}
}

// sectionHeader is the group label for an entry: "mode_id / group", or just
// "mode_id" when the entry sits directly under its source.
func sectionHeader(e *common.Entry) string {
	mid := e.ProviderIDValue()
	if mid == "" {
		mid = "—"
	}
	if g := e.GroupValue(); g != "" {
		return mid + " / " + g
	}
	return mid
}

// clampView re-validates the cursor against the current filtered length and
// derives the page from its display row (page = entryRow[cursor] / pageSize).
// page is never an independent source of truth, so navigation, filtering and
// terminal resize can never leave the highlighted row off the visible page
// (FR-009).
func (m *model) clampView() {
	m.cursor = pagination.ClampCursor(m.cursor, len(m.filtered))
	if m.pageSize < 1 || len(m.filtered) == 0 {
		m.page = 0
		return
	}
	m.page = m.entryRow[m.cursor] / m.pageSize
}

// moveByRows shifts the cursor by delta display rows (a page jump), snapping to
// the nearest selectable entry so header rows are skipped.
func (m *model) moveByRows(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	target := m.entryRow[m.cursor] + delta
	if target < 0 {
		target = 0
	}
	if last := len(m.rows) - 1; target > last {
		target = last
	}
	m.cursor = m.nearestEntry(target)
	m.clampView()
}

// nearestEntry returns the entry index of the closest entry row at or after
// rowIdx, else the closest before it.
func (m *model) nearestEntry(rowIdx int) int {
	for i := rowIdx; i < len(m.rows); i++ {
		if m.rows[i].header == "" {
			return m.rows[i].entryIdx
		}
	}
	for i := rowIdx - 1; i >= 0; i-- {
		if m.rows[i].header == "" {
			return m.rows[i].entryIdx
		}
	}
	return m.cursor
}

// containsFold reports a case-insensitive substring match.
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalFold(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func (m *model) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(m.spinner.Tick, scanCmd(m.svc), waitForResult(m.queue))
	}
	return waitForResult(m.queue)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = maxInt(1, msg.Width-4) // frame inner width
		m.pageSize = maxInt(1, m.height-8)    // list area: top, tabbar, install-header, [list], sep, status, sep, help, bottom (must match listView's rows)
		m.refreshFiltered()
		return m, nil
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	case jobDoneMsg:
		return m, m.handleJobDone(msg.Result)
	case scanDoneMsg:
		m.loading = false
		m.applyEntries(msg.entries)
		return m, waitForResult(m.queue)
	case spinner.TickMsg:
		if !m.loading {
			return m, nil // scan already finished: drop stray ticks so the spinner doesn't keep animating
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.searching {
		return m.handleSearchKey(msg)
	}
	if m.importing {
		return m.handleImportKey(msg)
	}
	if m.picker != nil {
		return m.handlePickerKey(msg)
	}
	if m.confirm != nil {
		return m.handleConfirmKey(msg)
	}
	if m.showTasks {
		return m.handleTasksKey(msg)
	}
	if m.targetWizard != nil {
		return m.handleTargetWizardKey(msg)
	}
	if m.showTargets {
		return m.handleTargetsKey(msg)
	}
	if m.showDetail {
		return m.handleDetailKey(msg)
	}
	return m.handleListKey(msg)
}

// openDetail builds and shows the full-screen detail page for the selected
// entry (Enter/v, tui-contract.md). The filesystem reads happen here, in
// Update, so View stays pure.
func (m *model) openDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.refreshDetail()
	m.showDetail = true
}

// refreshDetail rebuilds the detail page content and caches the FS-dependent
// install availability for the current entry. It runs in Update only (View is
// pure): the content, the kind-matching target count, and whether the entry is
// installed anywhere all come from the filesystem via the installer.
func (m *model) refreshDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.detail = m.buildDetail()
	e := m.filtered[m.cursor]
	targets := m.svc.Installer.Targets(e)
	installed := false
	for _, t := range targets {
		if m.svc.Installer.State(e, t) != common.InstallAbsent {
			installed = true
			break
		}
	}
	m.detailTargets = len(targets)
	m.detailInstalled = installed
	m.detailOffset = 0
}

func (m *model) clearSearch() {
	if m.search != "" {
		m.search = ""
		m.refreshFiltered()
	}
}

// installSelected opens one installs picker. Its checked state is the desired
// state for each target: checked means installed; unchecked means removed.
func (m *model) installSelected() {
	if m.cursor >= len(m.filtered) {
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusActive {
		m.status = fmt.Sprintf("%s is %s; only active entries can be installed", entry.Name, entry.Status)
		return
	}
	m.openInstallsPicker(entry)
}

// openInstallsPicker presents the entry's kind-matching targets. Each checkbox
// expresses the desired installation state, initialized from the real state.
func (m *model) openInstallsPicker(entry *common.Entry) {
	targets := m.svc.Installer.Targets(entry)
	if len(targets) == 0 {
		m.status = fmt.Sprintf("%s: no targets accept a %s", entry.Name, entry.Kind)
		return
	}
	items := make([]pages.PickerItem, len(targets))
	for i, t := range targets {
		state := m.svc.Installer.State(entry, t)
		items[i] = pages.PickerItem{Label: fmt.Sprintf("%s (%s)", t.Name, state), Value: t.Name, Checked: state == common.InstallInstalled}
	}
	name := entry.Name
	m.picker = &pages.Picker{
		Title: "Installs · " + name,
		Hint:  "[space] set installed  [enter] apply  [esc/q] cancel",
		Items: items,
		OnConfirm: func(sel []pages.PickerItem) {
			desired := make(map[string]bool, len(sel))
			for _, it := range sel {
				desired[it.Value] = true
			}
			var installTargets, uninstallTargets []string
			for _, target := range targets {
				state := m.svc.Installer.State(entry, target)
				if desired[target.Name] && state != common.InstallInstalled {
					installTargets = append(installTargets, target.Name)
				}
				if !desired[target.Name] && (state == common.InstallInstalled || state == common.InstallDangling) {
					uninstallTargets = append(uninstallTargets, target.Name)
				}
			}
			m.applyInstallChanges(name, installTargets, uninstallTargets)
		},
	}
}

func (m *model) applyInstallChanges(name string, installTargets, uninstallTargets []string) {
	if len(installTargets) == 0 && len(uninstallTargets) == 0 {
		m.status = "installs: no changes for " + name
		return
	}
	if len(uninstallTargets) == 0 {
		m.runInstallChanges(name, installTargets, nil)
		return
	}
	prompt := "Apply install changes for " + name + "?"
	if len(installTargets) > 0 {
		prompt += "\nInstall: " + strings.Join(installTargets, ", ")
	}
	prompt += "\nRemove: " + strings.Join(uninstallTargets, ", ")
	m.confirm = &pages.Confirm{Prompt: prompt, OnYes: func() {
		m.runInstallChanges(name, installTargets, uninstallTargets)
	}}
}

// runInstallChanges reconciles the selected targets with their requested
// install state. Removals run first and only managed installs are removed.
func (m *model) runInstallChanges(name string, installTargets, uninstallTargets []string) {
	attempt := func(force bool) func(ctx context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			var messages []string
			if len(uninstallTargets) > 0 {
				result, err := m.svc.Uninstall(ctx, name, services.InstallOptions{Targets: uninstallTargets})
				if err != nil {
					return nil, err
				}
				messages = append(messages, installStatusMessage("uninstall", name, result))
			}
			if len(installTargets) > 0 {
				result, err := m.svc.Install(ctx, name, services.InstallOptions{Targets: installTargets, Force: force})
				if err != nil {
					return nil, err
				}
				messages = append(messages, installStatusMessage("install", name, result))
			}
			return strings.Join(messages, "; "), nil
		}
	}
	if len(installTargets) > 0 {
		m.submitJobForce("installs "+name, attempt(false), attempt(true))
		return
	}
	m.submitJob("installs "+name, attempt(false))
}

// runInstall submits an install/uninstall job scoped to the chosen targets.
// An install that hits a same-named non-managed object at the destination
// fails with a needs-force error (Installer.Install); submitJobForce turns
// that into a confirm-then-retry offer instead of a dead end. Uninstall never
// needs force (it only ever removes a managed install, never a real file), so
// it runs as a plain job.
func (m *model) runInstall(action, name string, targets []string) {
	attempt := func(force bool) func(ctx context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			opts := services.InstallOptions{Targets: targets, Force: force}
			var result *services.InstallResult
			var err error
			if action == "install" {
				result, err = m.svc.Install(ctx, name, opts)
			} else {
				result, err = m.svc.Uninstall(ctx, name, opts)
			}
			if err != nil {
				return nil, err
			}
			return installStatusMessage(action, name, result), nil
		}
	}
	if action == "install" {
		m.submitJobForce(action+" "+name, attempt(false), attempt(true))
		return
	}
	m.submitJob(action+" "+name, attempt(false))
}

func installStatusMessage(action, name string, result *services.InstallResult) string {
	changed := 0
	for _, report := range result.Results {
		if report.Changed {
			changed++
		}
	}
	unchanged := len(result.Results) - changed
	if changed == 0 {
		return fmt.Sprintf("%s %s: no managed installs changed", action, name)
	}
	message := fmt.Sprintf("%sed %s across %d target(s)", action, name, changed)
	if unchanged > 0 {
		message += fmt.Sprintf("; %d unchanged", unchanged)
	}
	return message
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// clampInt bounds v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
