package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// installSkill creates the directory symlink for a skill in a skill target
// (install-semantics.md), idempotently.
func (i *Installer) installSkill(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name)
	switch stateLink(linkPath, entry.Path) {
	case common.InstallInstalled:
		return false, nil
	case common.InstallConflict, common.InstallDangling:
		if !force {
			return false, common.WithExitCode(
				common.WithNeedsForce(fmt.Errorf("install %q into %s: a same-named non-managed object exists; use --force", entry.Name, target.Name)),
				common.ExitObject)
		}
		if err := tx.BackupRemove(linkPath); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(target.Path, 0o755); err != nil {
		return false, fmt.Errorf("install %q: %w", entry.Name, err)
	}
	if err := tx.CreateLink(entry.Path, linkPath); err != nil {
		return false, err
	}
	return true, nil
}
