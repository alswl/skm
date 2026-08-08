package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// installClaudeMarkdown creates the <name>.md marker symlink for a command in
// a Claude-style command target (FR-015).
func (i *Installer) installClaudeMarkdown(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name+".md")
	switch stateLink(linkPath, entry.MarkerPath()) {
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
	if err := tx.CreateLink(entry.MarkerPath(), linkPath); err != nil {
		return false, err
	}
	return true, nil
}
