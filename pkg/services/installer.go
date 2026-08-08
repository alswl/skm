package services

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// Installer manages installs into kind-matching targets, dispatching on each
// target's declared InstallStrategy per kind — skill directory symlinks,
// command marker symlinks, command adapters, or a Target plugin
// (002-open-provider-target FR-013: no tool name drives this, only the
// target's own declaration; see common.InstallTarget.EffectiveStrategy for
// legacy-Kind compatibility). Install state is derived from the filesystem
// (or, for a plugin strategy, from the plugin), never stored (FR-019).
type Installer struct {
	targets       []common.InstallTarget
	targetPlugins map[string]*TargetPlugin
}

// New returns an Installer over the given targets. targetPlugins is the set
// of loaded Target plugins keyed by id, consulted when a target declares a
// "plugin:<id>" strategy; nil when no plugins are loaded.
func NewInstaller(targets []common.InstallTarget, targetPlugins map[string]*TargetPlugin) *Installer {
	return &Installer{targets: targets, targetPlugins: targetPlugins}
}

// pluginFor looks up a loaded Target plugin by strategy's plugin id, or
// returns a diagnosable error naming the target and the missing plugin
// (spec.md edge case: an incompatible/unresolvable strategy must be
// reported, not silently skipped).
func (i *Installer) pluginFor(strategy common.InstallStrategy, target common.InstallTarget) (*TargetPlugin, error) {
	id := strategy.PluginID()
	p := i.targetPlugins[id]
	if p == nil {
		return nil, common.WithExitCode(
			fmt.Errorf("target %q: plugin %q is not loaded", target.Name, id), common.ExitObject)
	}
	return p, nil
}

// Targets returns the kind-matching targets for an entry.
func (i *Installer) Targets(entry *common.Entry) []common.InstallTarget {
	var out []common.InstallTarget
	for _, t := range i.targets {
		if i.matches(entry, t) {
			out = append(out, t)
		}
	}
	return out
}

// TargetByName returns the target with the given name.
func (i *Installer) TargetByName(name string) (common.InstallTarget, bool) {
	for _, t := range i.targets {
		if t.Name == name {
			return t, true
		}
	}
	return common.InstallTarget{}, false
}

// Matches reports whether a target can receive an entry's kind, per the
// target's declared (or legacy-derived) Accepts.
func (i *Installer) Matches(entry *common.Entry, t common.InstallTarget) bool {
	return t.AcceptsKind(entry.Kind)
}

// matches is kept as an unexported alias for internal callers.
func (i *Installer) matches(entry *common.Entry, t common.InstallTarget) bool {
	return i.Matches(entry, t)
}

// Install installs entry into target idempotently, returning whether anything
// changed. Conflicts/dangling links are refused unless force is set
// (FR-014..FR-018). Dispatch is entirely by the target's declared strategy
// for entry.Kind — no tool name is consulted.
func (i *Installer) Install(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return false, nil
	}
	switch strategy {
	case common.StrategySkillSymlink:
		return i.installSkill(tx, entry, target, force)
	case common.StrategyCommandMarker:
		return i.installClaudeMarkdown(tx, entry, target, force)
	case common.StrategyCommandAdapter:
		return i.installAdapter(tx, entry, target, force)
	}
	if strategy.IsPlugin() {
		p, err := i.pluginFor(strategy, target)
		if err != nil {
			return false, err
		}
		return p.Install(entry, target, force)
	}
	return false, nil
}

// Uninstall removes only managed installs of entry from target, never a
// user's same-named real file/directory (FR-017).
func (i *Installer) Uninstall(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return false, nil
	}
	switch strategy {
	case common.StrategySkillSymlink:
		return i.uninstallSkill(tx, entry, target)
	case common.StrategyCommandMarker:
		return i.uninstallClaudeMarkdown(tx, entry, target)
	case common.StrategyCommandAdapter:
		return i.uninstallAdapter(tx, entry, target)
	}
	if strategy.IsPlugin() {
		p, err := i.pluginFor(strategy, target)
		if err != nil {
			return false, err
		}
		return p.Uninstall(entry, target)
	}
	return false, nil
}
