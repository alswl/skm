package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/utils/pagination"
)

// applyEntries installs a freshly scanned catalog and recomputes the in-memory
// catalog state used by the list page.
func (m *model) applyEntries(entries []*common.Entry, states installStates) {
	m.entries = entries
	m.installs = states
	m.computeProviderTabs()
	m.refreshFiltered()
}

func (m *model) computeProviderTabs() {
	seen := map[string]bool{}
	hasNone := false
	for _, e := range m.entries {
		if id := e.ProviderIDValue(); id != "" {
			seen[id] = true
		} else {
			hasNone = true
		}
	}
	names := make([]string, 0, len(seen))
	for id := range seen {
		names = append(names, id)
	}
	sort.Strings(names)
	tabs := append([]string{tabAll}, names...)
	if hasNone {
		tabs = append(tabs, tabNone)
	}
	m.providerTabs = tabs
	if m.providerTabIdx >= len(tabs) {
		m.providerTabIdx = 0
	}
}

func (m *model) cycleProviderTab(delta int) {
	if len(m.providerTabs) == 0 {
		return
	}
	n := len(m.providerTabs)
	m.providerTabIdx = ((m.providerTabIdx+delta)%n + n) % n
	m.refreshFiltered()
}

func isDigitKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] >= '0' && msg.Runes[0] <= '9'
}

func (m *model) jumpToProviderTab(n int) {
	if n < 0 || n >= len(m.providerTabs) {
		return
	}
	m.providerTabIdx = n
	m.refreshFiltered()
}

func (m *model) activeProviderTab() string {
	if m.providerTabIdx <= 0 || m.providerTabIdx >= len(m.providerTabs) {
		return tabAll
	}
	return m.providerTabs[m.providerTabIdx]
}

// matchesProviderTab reports whether entry e is visible in provider tab tab
// (the same rule the list applies in refreshFiltered, so batch-update's scope
// matches what the current tab shows).
func matchesProviderTab(e *common.Entry, tab string) bool {
	if tab == tabAll {
		return true
	}
	id := e.ProviderIDValue()
	if tab == tabNone {
		return id == ""
	}
	return id == tab
}

func (m *model) refreshFiltered() {
	q, tab := m.search, m.activeProviderTab()
	m.filtered = nil
	for _, e := range m.entries {
		if !m.showArchived && e.Status == common.StatusArchived {
			continue
		}
		if !matchesProviderTab(e, tab) {
			continue
		}
		if q != "" && !containsFold(e.Name, q) && !containsFold(e.Description, q) {
			continue
		}
		m.filtered = append(m.filtered, e)
	}
	m.filteredHaveGroups = false
	for _, e := range m.filtered {
		if e.GroupValue() != "" {
			m.filteredHaveGroups = true
			break
		}
	}
	m.buildRows()
	m.clampView()
}

type dispRow struct {
	header   string
	entryIdx int
}

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
	for i := range m.filtered {
		m.entryRow[i] = len(m.rows)
		m.rows = append(m.rows, dispRow{entryIdx: i})
	}
}

func sectionHeader(e *common.Entry) string {
	id := e.ProviderIDValue()
	if id == "" {
		id = "—"
	}
	if group := e.GroupValue(); group != "" {
		return id + " / " + group
	}
	return id
}

func (m *model) clampView() {
	m.cursor = pagination.ClampCursor(m.cursor, len(m.filtered))
	if m.pageSize < 1 || len(m.filtered) == 0 {
		m.page = 0
		return
	}
	m.page = m.entryRow[m.cursor] / m.pageSize
}

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
	for i := range a {
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
