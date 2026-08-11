package services

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
)

// State derives the install health of entry within target from the
// filesystem (FR-019): absent / installed / conflict / dangling. Dispatch
// mirrors Install/Uninstall: by the target's declared strategy for entry.Kind.
func (i *Installer) State(entry *common.Entry, target common.InstallTarget) common.InstallState {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return common.InstallAbsent
	}
	switch strategy {
	case common.StrategySkillSymlink:
		return engines.StateLink(filepath.Join(target.Path, entry.Name), entry.Path)
	case common.StrategyCommandSymlink:
		return engines.StateLink(filepath.Join(target.Path, entry.Name), entry.Path)
	case common.StrategyCommandMarker:
		return engines.StateLink(filepath.Join(target.Path, entry.Name+".md"), entry.MarkerPath())
	case common.StrategyCommandAdapter:
		return engines.StateAdapter(filepath.Join(target.Path, entry.Name), entry)
	}
	driver, err := i.driverFor(strategy, target)
	if err != nil {
		return common.InstallAbsent
	}
	state, err := driver.State(entry, target)
	if err != nil {
		return common.InstallAbsent
	}
	return state
}
