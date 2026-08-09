package tui

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/alswl/skm/skm/pkg/tui/components"
)

// installCell is one target's install state for a single entry: the target's
// name (which also drives the list column's header label and width) and the
// state itself.
type installCell struct {
	name  string
	state common.InstallState
}

// installStates is the model's in-memory view of install health: for every
// scanned entry, one cell per configured target in Cfg.Targets order.
//
// It is the single source of truth every screen reads — the list columns, the
// detail page, the installs picker, the actions menu and the fix action all
// answer "is this installed / conflicting / dangling?" from here, and none of
// them touches the filesystem or a target plugin to find out. That rule is not
// stylistic: a probe is a filesystem read and, for a plugin-backed target, a
// subprocess round-trip, and every one of those callers runs on the Bubble Tea
// event loop, where the cost stops rendering and input outright
// (TestKeypathsNeverBlockTheEventLoop).
//
// The map is only ever built by scanInstallStates on a tea.Cmd goroutine
// (scanCmd) and swapped in wholesale by applyEntries, so it always
// describes the same scan as m.entries.
type installStates map[string][]installCell

// forEntry returns the targets that can actually receive the named entry,
// paired with their current state. scanInstallStates records a cell for every
// configured target so the list can keep its columns aligned, marking the ones
// that structurally cannot take the entry's kind components.InstallNA; those
// are not real choices, so nothing outside the list columns should see them.
func (s installStates) forEntry(name string) []installCell {
	cells := s[name]
	out := make([]installCell, 0, len(cells))
	for _, c := range cells {
		if c.state != components.InstallNA {
			out = append(out, c)
		}
	}
	return out
}

// installedAnywhere reports whether the entry is present in at least one
// target, in any state other than absent.
func (s installStates) installedAnywhere(name string) bool {
	for _, c := range s.forEntry(name) {
		if c.state != common.InstallAbsent {
			return true
		}
	}
	return false
}

// broken returns the targets whose install is repairable — a conflict (a
// non-managed object occupies the destination) or a dangling link. Both are
// fixed the same way, by force-installing over them (see fixSelected).
func (s installStates) broken(name string) []string {
	var out []string
	for _, c := range s.forEntry(name) {
		switch c.state {
		case common.InstallConflict, common.InstallDangling:
			out = append(out, c.name)
		}
	}
	return out
}

// scanInstallStates derives the install state of every entry in every
// configured target (FR-041). It is the one and only place that probes for
// install health, and it must only ever run on a tea.Cmd goroutine — see
// installStates for why. It is a free function rather than a model method
// precisely so it cannot be reached from Update by accident.
func scanInstallStates(svc *services.Services, entries []*common.Entry) installStates {
	targets := svc.Cfg.Targets
	states := make(installStates, len(entries))
	for _, e := range entries {
		// Evaluated for every entry (not just active ones) so a non-standard or
		// moved entry's dangling/conflicting installs are still visible in the
		// list ("安装状态无法在 list 页面看到").
		cells := make([]installCell, len(targets))
		for i, t := range targets {
			state := components.InstallNA
			if t.AcceptsKind(e.Kind) {
				state = svc.Installer.State(e, t)
			}
			cells[i] = installCell{name: t.Name, state: state}
		}
		states[e.Name] = cells
	}
	return states
}
