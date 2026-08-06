package installer

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// Installer manages installs into kind-matching targets: skill directory
// symlinks, Claude command marker symlinks, and Codex/CodeFuse command
// adapters. Install state is derived from the filesystem, never stored
// (FR-019).
type Installer struct {
	targets []common.InstallTarget
}

// New returns an Installer over the given targets.
func New(targets []common.InstallTarget) *Installer {
	return &Installer{targets: targets}
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

// Matches reports whether a target can receive an entry: a skill goes to
// skill targets; a command goes to command (Claude) AND skill (Codex/CodeFuse
// adapter) targets.
func (i *Installer) Matches(entry *common.Entry, t common.InstallTarget) bool {
	switch entry.Kind {
	case common.KindSkill:
		return t.Kind == common.KindSkill
	case common.KindCommand:
		return true
	}
	return false
}

// matches is kept as an unexported alias for internal callers.
func (i *Installer) matches(entry *common.Entry, t common.InstallTarget) bool {
	return i.Matches(entry, t)
}

// Install installs entry into target idempotently, returning whether anything
// changed. Conflicts/dangling links are refused unless force is set
// (FR-014..FR-018).
func (i *Installer) Install(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	switch entry.Kind {
	case common.KindSkill:
		if target.Kind != common.KindSkill {
			return false, nil
		}
		return i.installSkill(tx, entry, target, force)
	case common.KindCommand:
		if target.Kind == common.KindCommand {
			return i.installClaudeMarkdown(tx, entry, target, force)
		}
		return i.installAdapter(tx, entry, target, force)
	}
	return false, nil
}

// Uninstall removes only managed installs of entry from target, never a
// user's same-named real file/directory (FR-017).
func (i *Installer) Uninstall(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	switch entry.Kind {
	case common.KindSkill:
		if target.Kind != common.KindSkill {
			return false, nil
		}
		return i.uninstallSkill(tx, entry, target)
	case common.KindCommand:
		if target.Kind == common.KindCommand {
			return i.uninstallClaudeMarkdown(tx, entry, target)
		}
		return i.uninstallAdapter(tx, entry, target)
	}
	return false, nil
}
