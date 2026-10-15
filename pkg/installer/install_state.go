package installer

import (
	"os"
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

// LegacyForkTargets returns skill targets whose slot still points at a fork
// path moved by the v1-to-v2 migration. This is deliberately narrower than a
// dangling-link scan: only an exact managed old target may be replaced.
func (i *Installer) LegacyForkTargets(entry *common.Entry, oldPath string) []common.InstallTarget {
	var out []common.InstallTarget
	for _, target := range i.targets {
		strategy, ok := target.EffectiveStrategy(entry.Kind)
		if !ok || strategy != common.StrategySkillSymlink || !i.Matches(entry, target) {
			continue
		}
		link := filepath.Join(target.Path, entry.Name)
		value, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(link), value)
		}
		if filepath.Clean(value) == filepath.Clean(oldPath) {
			out = append(out, target)
		}
	}
	return out
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
