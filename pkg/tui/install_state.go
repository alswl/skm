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
// scanned entry, one cell per configured target in Cfg.Targets order. Every
// screen answers "installed / conflicting / dangling?" from here and never
// probes the filesystem — a probe would block the Bubble Tea event loop
// (TestKeypathsNeverBlockTheEventLoop). It is keyed by the entry's path (its
// identity), so a same-named active/archived pair never shares cells, and is
// only ever built by scanInstallStates on a tea.Cmd goroutine (scanCmd),
// swapped in wholesale by applyEntries with the same scan as m.entries.
type installStates map[string][]installCell

// forEntry returns the targets that can actually receive the entry at path,
// paired with their current state. scanInstallStates records a cell for every
// configured target so the list can keep its columns aligned, marking the ones
// that structurally cannot take the entry's kind components.InstallNA; those
// are not real choices, so nothing outside the list columns should see them.
func (s installStates) forEntry(path string) []installCell {
	cells := s[path]
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
func (s installStates) installedAnywhere(path string) bool {
	for _, c := range s.forEntry(path) {
		if c.state != common.InstallAbsent {
			return true
		}
	}
	return false
}

// broken returns the targets whose install is repairable — a conflict (a
// non-managed object occupies the destination) or a dangling link. Both are
// fixed the same way, by force-installing over them (see fixSelected).
func (s installStates) broken(path string) []string {
	var out []string
	for _, c := range s.forEntry(path) {
		switch c.state {
		case common.InstallConflict, common.InstallDangling:
			out = append(out, c.name)
		}
	}
	return out
}

// archivedInstallCells renders one n/a cell per configured target so an
// archived row stays column-aligned without inventing an install state.
func archivedInstallCells(targets []common.InstallTarget) []installCell {
	cells := make([]installCell, 0, len(targets))
	for _, t := range targets {
		cells = append(cells, installCell{name: t.Name, state: components.InstallNA})
	}
	return cells
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
		// Archived entries are never installed (models.go): their name's target
		// link belongs to a same-named active entry, so probing would report a
		// phantom dangling and offer a non-existent fix.
		if e.Status == common.StatusArchived {
			continue
		}
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
		states[e.Path] = cells
	}
	return states
}
