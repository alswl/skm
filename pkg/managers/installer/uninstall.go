package installer

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// uninstallSkill removes the managed directory symlink of a skill, leaving any
// non-managed same-named object untouched (FR-017).
func (i *Installer) uninstallSkill(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name)
	if !isManagedLink(linkPath, entry.Path) {
		return false, nil
	}
	if err := tx.RemoveManaged(linkPath); err != nil {
		return false, err
	}
	return true, nil
}

// uninstallClaudeMarkdown removes the managed <name>.md marker symlink.
func (i *Installer) uninstallClaudeMarkdown(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name+".md")
	if !isManagedLink(linkPath, entry.MarkerPath()) {
		return false, nil
	}
	if err := tx.RemoveManaged(linkPath); err != nil {
		return false, err
	}
	return true, nil
}

// uninstallAdapter removes the managed adapter directory for a command.
func (i *Installer) uninstallAdapter(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	adapterDir := filepath.Join(target.Path, entry.Name)
	if !isManagedAdapter(adapterDir, entry) {
		return false, nil
	}
	if err := tx.RemoveManaged(adapterDir); err != nil {
		return false, err
	}
	return true, nil
}

// isManagedLink reports whether linkPath is a symlink resolving to
// expectedTarget (i.e. it is "ours").
func isManagedLink(linkPath, expectedTarget string) bool {
	if !dal.IsSymlink(linkPath) {
		return false
	}
	return dal.ResolveLink(linkPath) == expectedTarget
}
