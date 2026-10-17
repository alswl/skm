package installer

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

// RefreshTargets returns the entry's kind-matching targets that currently
// hold a managed install a content update should refresh in place: healthy
// installs, plus managed command adapters the update just made stale (an
// adapter keeps a copy of the entry marker). Absent, dangling and
// foreign-conflict targets are excluded — claiming those is an explicit
// install's job, never a side effect of an update.
func (i *Installer) RefreshTargets(entry *common.Entry) []common.InstallTarget {
	var out []common.InstallTarget
	for _, t := range i.targets {
		if !i.Matches(entry, t) || nameRuleRejects(entry, t) {
			continue
		}
		switch i.State(entry, t) {
		case common.InstallInstalled:
			out = append(out, t)
		case common.InstallConflict:
			if strategy, ok := t.EffectiveStrategy(entry.Kind); ok &&
				strategy == common.StrategyCommandAdapter &&
				engines.IsStaleManagedAdapter(filepath.Join(t.Path, entry.Name), entry) {
				out = append(out, t)
			}
		}
	}
	return out
}
