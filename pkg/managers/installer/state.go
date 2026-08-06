package installer

import (
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// State derives the install health of entry within target from the
// filesystem (FR-019): absent / installed / conflict / dangling.
func (i *Installer) State(entry *common.Entry, target common.InstallTarget) common.InstallState {
	switch entry.Kind {
	case common.KindSkill:
		if target.Kind != common.KindSkill {
			return common.InstallAbsent
		}
		return stateLink(filepath.Join(target.Path, entry.Name), entry.Path)
	case common.KindCommand:
		if target.Kind == common.KindCommand {
			return stateLink(filepath.Join(target.Path, entry.Name+".md"), entry.MarkerPath())
		}
		return stateAdapter(filepath.Join(target.Path, entry.Name), entry)
	}
	return common.InstallAbsent
}

// stateLink classifies a symlink target by presence/resolution.
func stateLink(linkPath, expectedTarget string) common.InstallState {
	if dal.PathExists(linkPath) {
		if dal.IsSymlink(linkPath) {
			if dal.ResolveLink(linkPath) == expectedTarget {
				return common.InstallInstalled
			}
			return common.InstallDangling
		}
		return common.InstallConflict
	}
	if dal.IsSymlink(linkPath) {
		// broken symlink: lstat succeeds, stat fails
		return common.InstallDangling
	}
	return common.InstallAbsent
}

// stateAdapter classifies a command's managed adapter directory.
func stateAdapter(adapterDir string, entry *common.Entry) common.InstallState {
	if dal.PathExists(adapterDir) {
		if dal.IsSymlink(adapterDir) {
			if dal.ResolveLink(adapterDir) == entry.Path {
				return common.InstallInstalled
			}
			return common.InstallDangling
		}
		if isManagedAdapter(adapterDir, entry) {
			return common.InstallInstalled
		}
		return common.InstallConflict
	}
	if dal.IsSymlink(adapterDir) {
		return common.InstallDangling
	}
	return common.InstallAbsent
}

// isManagedAdapter reports whether dir is a managed adapter for entry: it
// carries the adapter marker and its SKILL.md link resolves to the entry's
// marker (install-semantics.md).
func isManagedAdapter(dir string, entry *common.Entry) bool {
	if !dal.PathExists(filepath.Join(dir, dal.AdapterMarker)) {
		return false
	}
	return dal.ResolveLink(filepath.Join(dir, "SKILL.md")) == entry.MarkerPath()
}
