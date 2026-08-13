package services

import (
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
	if !strategy.IsPlugin() {
		state, err := engines.State(strategy, entry, target)
		if err != nil {
			return common.InstallAbsent
		}
		return state
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
