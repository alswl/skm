package services

import (
	"context"
	"fmt"
	"slices"

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
	targetPlugins map[string]TargetDriver
}

// Diff delegates a target-side comparison to the same driver that owns its
// install semantics. This removes UI knowledge of strategy-specific paths.
func (i *Installer) Diff(ctx context.Context, entry *common.Entry, target common.InstallTarget) (string, error) {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return "", nil
	}
	driver, err := i.driverFor(strategy, target)
	if err != nil {
		return "", err
	}
	return driver.Diff(ctx, entry, target)
}

// OrphanDangling enumerates target-side dangling links whose names do not
// identify an active repository entry. Built-in and plugin strategies expose
// this through the same TargetDriver inspection contract.
func (i *Installer) OrphanDangling(ctx context.Context, entries []*common.Entry) ([]DanglingInstall, error) {
	active := map[string]bool{}
	for _, e := range entries {
		if e.Status == common.StatusActive {
			active[e.Name] = true
		}
	}
	seen := map[string]bool{}
	var out []DanglingInstall
	for _, target := range i.targets {
		for _, kind := range target.EffectiveAccepts() {
			strategy, ok := target.EffectiveStrategy(kind)
			if !ok {
				continue
			}
			key := target.Name + "\x00" + string(strategy)
			if seen[key] {
				continue
			}
			seen[key] = true
			driver, err := i.driverFor(strategy, target)
			if err != nil {
				return nil, err
			}
			items, err := driver.Inspect(ctx, target)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if !active[item.Name] {
					out = append(out, item)
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b DanglingInstall) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	return out, nil
}

func (i *Installer) CleanDangling(ctx context.Context, item DanglingInstall) error {
	target, ok := i.TargetByName(item.TargetName)
	if !ok {
		return fmt.Errorf("fix: target %q no longer exists", item.TargetName)
	}
	driver, err := i.driverFor(item.Strategy, target)
	if err != nil {
		return err
	}
	return driver.RepairDangling(ctx, item, target)
}

// New returns an Installer over the given targets. targetPlugins is the set
// of loaded Target plugins keyed by id, consulted when a target declares a
// "plugin:<id>" strategy; nil when no plugins are loaded.
func NewInstaller(targets []common.InstallTarget, targetPlugins map[string]TargetDriver) *Installer {
	return &Installer{targets: targets, targetPlugins: targetPlugins}
}

// pluginFor looks up a loaded Target plugin by strategy's plugin id, or
// returns a diagnosable error naming the target and the missing plugin
// (spec.md edge case: an incompatible/unresolvable strategy must be
// reported, not silently skipped).
func (i *Installer) pluginFor(strategy common.InstallStrategy, target common.InstallTarget) (TargetDriver, error) {
	id := strategy.PluginID()
	p := i.targetPlugins[id]
	if p == nil {
		return nil, common.WithExitCode(
			fmt.Errorf("target %q: plugin %q is not loaded", target.Name, id), common.ExitObject)
	}
	return p, nil
}

func (i *Installer) driverFor(strategy common.InstallStrategy, target common.InstallTarget) (TargetDriver, error) {
	if strategy.IsPlugin() {
		return i.pluginFor(strategy, target)
	}
	switch strategy {
	case common.StrategySkillSymlink, common.StrategyCommandMarker, common.StrategyCommandAdapter:
		return builtinTargetDriver{strategy: strategy}, nil
	}
	return nil, fmt.Errorf("target %q: unknown strategy %q", target.Name, strategy)
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
	driver, err := i.driverFor(strategy, target)
	if err != nil {
		return false, err
	}
	return driver.Install(tx, entry, target, force)
}

// Uninstall removes only managed installs of entry from target, never a
// user's same-named real file/directory (FR-017).
func (i *Installer) Uninstall(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return false, nil
	}
	driver, err := i.driverFor(strategy, target)
	if err != nil {
		return false, err
	}
	return driver.Uninstall(tx, entry, target)
}

// RemoveForeign removes the non-managed object blocking entry's target
// (InstallConflict), restoring it to absent — the inverse of a force install.
// Only a re-verified conflict is ever touched; a managed install or dangling
// link is Uninstall's job (FR-017). Dispatch mirrors Install/Uninstall.
func (i *Installer) RemoveForeign(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	strategy, ok := target.EffectiveStrategy(entry.Kind)
	if !ok {
		return false, nil
	}
	driver, err := i.driverFor(strategy, target)
	if err != nil {
		return false, err
	}
	return driver.RemoveForeign(tx, entry, target)
}
