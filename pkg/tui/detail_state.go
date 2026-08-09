package tui

// openDetail prepares and shows the selected entry's full-screen detail page.
func (m *model) openDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.refreshDetail()
	m.showDetail = true
}

// refreshDetail performs the filesystem-backed detail construction during
// Update, keeping View pure and the availability data cached.
func (m *model) refreshDetail() {
	if m.cursor >= len(m.filtered) {
		return
	}
	m.detail = m.buildDetail()
	e := m.filtered[m.cursor]
	m.detailTargets = len(m.installs.forEntry(e.Name))
	m.detailInstalled = m.installs.installedAnywhere(e.Name)
	m.detailOffset = 0
}
