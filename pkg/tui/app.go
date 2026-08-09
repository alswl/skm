package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	isatty "github.com/mattn/go-isatty"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/jobs"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/components"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
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
	ctx      context.Context
	svc      *services.Services
	queue    *jobs.Queue
	entries  []*common.Entry // full scan result
	filtered []*common.Entry // after search filter, sorted by source
	rows     []dispRow       // display rows: section headers interleaved with entries
	entryRow []int           // entryRow[i] = index into rows for filtered[i]
	// filteredHaveGroups is whether any currently-visible entry carries a
	// group, cached alongside filtered because showGroupColumn asks it up to
	// four times per frame from View (header row, entry rows, and twice more
	// via showInstallColumns) and answering it means walking every filtered
	// entry — the same reason install state is derived in Update, not View.
	filteredHaveGroups bool
	cursor             int // index into filtered (selectable entries only)
	page               int // 0-based page over display rows
	pageSize           int
	search             string
	searching          bool
	showArchived       bool // archived entries are hidden until toggled with `.`
	importing          bool
	importAddr         string
	status             string    // last action status line
	statusSetAt        time.Time // when status was last set to a non-empty value; drives statusAutoHide
	width              int
	height             int

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
	picker       *pages.Picker        // active multi/single-select modal (nil when closed)
	confirm      *pages.Confirm       // active confirmation modal (nil when closed)
	showTasks    bool                 // task-center view (J)
	tasksCursor  int                  // cursor into the flattened task list
	installs     installStates        // the model-owned view of install health; see install_state.go
	forceRetries map[int64]forceRetry // job id -> force-retry offer, for jobs that failed needing --force

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

// statusAutoHide is how long a one-off status-line notification (e.g.
// "archived shown", a rejection reason, a job result) stays visible before
// clearing itself, so it never sits stale forever if the user doesn't happen
// to press a key afterward (handleKey already clears it on the next
// keypress; this covers the case where there isn't one).
const statusAutoHide = 3 * time.Second

// setStatus is the single place that sets the transient status-line message
// (every "m.status = …" assignment in the package goes through this): it
// records when the message was set so the recurring statusTick can auto-hide
// it after statusAutoHide.
func (m *model) setStatus(s string) {
	m.status = s
	m.statusSetAt = time.Now()
}

// statusTickMsg drives the periodic check that auto-hides a stale status
// message (statusAutoHide). It ticks for the whole program lifetime, like
// waitForResult's listener loop — status can be set from far outside any key
// handler (a background job's result, jobs_wire.go), so a self-perpetuating
// tick is simpler and more robust than trying to arm/disarm a one-shot timer
// from every call site that can set or clear m.status.
type statusTickMsg struct{}

// statusTickInterval is how often statusTickMsg fires — frequent enough that
// the 3s auto-hide feels prompt, infrequent enough to be free.
const statusTickInterval = 250 * time.Millisecond

func statusTickCmd() tea.Cmd {
	return tea.Tick(statusTickInterval, func(time.Time) tea.Msg { return statusTickMsg{} })
}

// scanReason says what occasioned a scan. The scan itself is identical in all
// three cases; only what applying the result does *besides* installing the
// entries differs (clear the loading spinner / report a count), so this is a
// field rather than three parallel message types and commands.
type scanReason int

const (
	scanInitial  scanReason = iota // startup, behind the loading spinner
	scanAfterJob                   // a background job finished and may have changed things
	scanManual                     // the user pressed R and is owed an answer
)

// scanDoneMsg carries a completed background scan and the install states
// derived from it.
type scanDoneMsg struct {
	reason  scanReason
	entries []*common.Entry
	cols    installStates
}

// scanCmd runs the potentially slow filesystem scan off the Bubble Tea event
// loop, so the program renders immediately instead of blocking before the
// first frame (startup), and stays responsive to the task center, cancel and
// navigation while a rescan runs (after a job, or on R). It derives the
// install states here too, for the same reason and because they are the far
// more expensive half.
func scanCmd(svc *services.Services, reason scanReason) tea.Cmd {
	return func() tea.Msg {
		entries := svc.Scan()
		return scanDoneMsg{reason: reason, entries: entries, cols: scanInstallStates(svc, entries)}
	}
}

// computeProviderTabs derives the provider filter tabs from the current scan:
// tabAll, then every distinct mode_id in sorted order, then tabNone if any
// entry has no mode_id. Called after every scan (startup and job completion),
// like scanInstallStates — but from applyEntries, since it is pure in-memory work
// and cheap enough for the event loop.
func (m *model) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(m.spinner.Tick, scanCmd(m.svc, scanInitial), waitForResult(m.queue), statusTickCmd())
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
		// handleJobDone only returns the follow-up rescan (or nil); this is
		// the one place that re-arms the queue's result listener, so each job
		// completion keeps exactly one listener alive (see
		// TestScanDoneMsgDoesNotDoubleArmResultListener).
		return m, tea.Batch(m.handleJobDone(msg.Result), waitForResult(m.queue))
	case scanDoneMsg:
		if msg.reason == scanInitial {
			m.loading = false
		}
		m.applyEntries(msg.entries, msg.cols)
		if m.showDetail && m.cursor < len(m.filtered) {
			m.refreshDetail() // the entry may have moved/installed; show the new state
		}
		if msg.reason == scanManual {
			m.setStatus(fmt.Sprintf("refreshed: %d entries", len(msg.entries)))
		}
		// No listener is armed here: Init started the one persistent
		// job-result listener, and jobDoneMsg re-arms it (see
		// TestScanDoneMsgDoesNotDoubleArmResultListener).
		return m, nil
	case statusTickMsg:
		if m.status != "" && time.Since(m.statusSetAt) >= statusAutoHide {
			m.status = ""
		}
		return m, statusTickCmd()
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

// handleKey dispatches a keypress to the active mode's handler. It first
// drops any stale one-off notification (e.g. "archived shown", a rejection
// reason) left over from a previous keypress: earlier this was only cleared
// on list-view cursor movement, so any other next action (opening detail,
// switching tabs, opening a picker, …) left it stuck in the status bar
// indefinitely instead of reverting to the current selection summary.
func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
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
		m.setStatus(fmt.Sprintf("%s is %s; only active entries can be installed", entry.Name, entry.Status))
		return
	}
	m.openInstallsPicker(entry)
}

// openInstallsPicker presents the entry's kind-matching targets. Each checkbox
// expresses the desired installation state, initialized from the real state.
func (m *model) openInstallsPicker(entry *common.Entry) {
	cells := m.installs.forEntry(entry.Name)
	if len(cells) == 0 {
		m.setStatus(fmt.Sprintf("%s: no targets accept a %s", entry.Name, entry.Kind))
		return
	}
	items := make([]pages.PickerItem, len(cells))
	for i, c := range cells {
		items[i] = pages.PickerItem{Label: fmt.Sprintf("%s (%s)", c.name, c.state), Value: c.name, Checked: c.state == common.InstallInstalled}
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
			// cells is the state the picker was drawn from, so what gets applied
			// is exactly what the user was shown — and, like everywhere else,
			// costs no probe on the event loop.
			var installTargets, uninstallTargets []string
			for _, c := range cells {
				if desired[c.name] && c.state != common.InstallInstalled {
					installTargets = append(installTargets, c.name)
				}
				if !desired[c.name] && (c.state == common.InstallInstalled || c.state == common.InstallDangling) {
					uninstallTargets = append(uninstallTargets, c.name)
				}
			}
			m.applyInstallChanges(name, installTargets, uninstallTargets)
		},
	}
}

func (m *model) applyInstallChanges(name string, installTargets, uninstallTargets []string) {
	if len(installTargets) == 0 && len(uninstallTargets) == 0 {
		m.setStatus("installs: no changes for " + name)
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
