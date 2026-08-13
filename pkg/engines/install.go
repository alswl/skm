package engines

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// InstallSkill creates the directory symlink for a skill in a skill target
// (install-semantics.md), idempotently.
func InstallSkill(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	return InstallDirectory(tx, entry, target, force)
}

// InstallDirectory creates a symlink from the target slot to the complete
// entry directory. It is used for skill entries and targets whose declared
// command strategy explicitly supports directory-shaped commands.
func InstallDirectory(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	if fi, err := os.Stat(entry.Path); err != nil || !fi.IsDir() {
		if err == nil {
			err = fmt.Errorf("entry is not a directory")
		}
		return false, fmt.Errorf("install %q: %w", entry.Name, err)
	}
	linkPath := filepath.Join(target.Path, installSlot(entry))
	switch StateLink(linkPath, entry.Path) {
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

// UninstallDirectory removes a managed directory symlink.
func UninstallDirectory(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	linkPath := filepath.Join(target.Path, installSlot(entry))
	if !IsManagedLink(linkPath, entry.Path) {
		return false, nil
	}
	if err := tx.RemoveManaged(linkPath); err != nil {
		return false, err
	}
	return true, nil
}

// installSlot preserves an adopted source's target-side directory name.
func installSlot(entry *common.Entry) string {
	if entry.Origin != nil && entry.Origin.InstallSlot != "" {
		return entry.Origin.InstallSlot
	}
	return entry.Name
}

// InstallClaudeMarkdown creates the <name>.md marker symlink for a command in
// a Claude-style command target (FR-015).
func InstallClaudeMarkdown(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name+".md")
	switch StateLink(linkPath, entry.MarkerPath()) {
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

// InstallAdapter creates a managed skill-form adapter directory for a command
// in a target whose declared strategy for command is command-adapter
// (FR-016): a regular SKILL.md copy of the command marker plus links to auxiliary
// resources, so the command is discovered as a skill by that target's tool.
func InstallAdapter(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	adapterDir := filepath.Join(target.Path, entry.Name)
	switch StateAdapter(adapterDir, entry) {
	case common.InstallInstalled:
		return false, nil
	case common.InstallConflict, common.InstallDangling:
		if !force {
			return false, common.WithExitCode(
				common.WithNeedsForce(fmt.Errorf("install %q into %s: a same-named non-managed object exists; use --force", entry.Name, target.Name)),
				common.ExitObject)
		}
		if err := tx.BackupRemove(adapterDir); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(adapterDir, 0o755); err != nil {
		return false, fmt.Errorf("install %q: %w", entry.Name, err)
	}
	if err := tx.CopyFile(entry.MarkerPath(), filepath.Join(adapterDir, "SKILL.md")); err != nil {
		return false, err
	}
	resources, err := EntryResources(entry)
	if err != nil {
		return false, err
	}
	for _, r := range resources {
		if err := tx.CreateLink(filepath.Join(entry.Path, r), filepath.Join(adapterDir, r)); err != nil {
			return false, err
		}
	}
	if err := os.WriteFile(filepath.Join(adapterDir, dal.AdapterMarker), []byte("skm managed command adapter\n"), 0o644); err != nil {
		return false, fmt.Errorf("install %q: write adapter marker: %w", entry.Name, err)
	}
	return true, nil
}

// EntryResources lists the auxiliary files in an entry directory that must
// stay visible through the adapter (all files except the marker and
// meta.json). A single-file command (entry.Path is the .md marker itself, per
// Entry.MarkerPath) has no sibling directory to scan and so has no resources.
func EntryResources(entry *common.Entry) ([]string, error) {
	if entry.Kind == common.KindCommand && strings.HasSuffix(entry.Path, ".md") {
		return nil, nil
	}
	children, err := os.ReadDir(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("list resources for %q: %w", entry.Name, err)
	}
	var out []string
	for _, c := range children {
		if c.Name() == entry.Kind.MarkerFile() || c.Name() == "meta.json" {
			continue
		}
		out = append(out, c.Name())
	}
	return out, nil
}

// UninstallSkill removes the managed directory symlink of a skill, leaving any
// non-managed same-named object untouched (FR-017).
func UninstallSkill(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	linkPath := filepath.Join(target.Path, installSlot(entry))
	if !IsManagedLink(linkPath, entry.Path) && !IsLegacySelfBuildLink(linkPath, entry) {
		return false, nil
	}
	if err := tx.RemoveManaged(linkPath); err != nil {
		return false, err
	}
	return true, nil
}

// IsLegacySelfBuildLink recognizes the exact pre-provider-layout symlink that
// skm used for self-built entries: <root>/skills/<name>. Current self-built
// entries live under <root>/skills/self-build/<name>. This narrow comparison
// permits uninstall to clean up its own old dangling links without ever
// removing an arbitrary user-created dangling symlink.
func IsLegacySelfBuildLink(linkPath string, entry *common.Entry) bool {
	if entry.ProviderIDValue() != "self-build" || !dal.IsSymlink(linkPath) {
		return false
	}
	providerDir := filepath.Dir(entry.Path)
	if filepath.Base(providerDir) != "self-build" {
		return false
	}
	legacyPath := filepath.Join(filepath.Dir(providerDir), filepath.Base(entry.Path))
	return dal.ResolveLink(linkPath) == legacyPath
}

// UninstallClaudeMarkdown removes the managed <name>.md marker symlink.
func UninstallClaudeMarkdown(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	linkPath := filepath.Join(target.Path, entry.Name+".md")
	if !IsManagedLink(linkPath, entry.MarkerPath()) {
		return false, nil
	}
	if err := tx.RemoveManaged(linkPath); err != nil {
		return false, err
	}
	return true, nil
}

// UninstallAdapter removes the managed adapter directory for a command.
func UninstallAdapter(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget) (bool, error) {
	adapterDir := filepath.Join(target.Path, entry.Name)
	if !IsManagedAdapter(adapterDir, entry) {
		return false, nil
	}
	if err := tx.RemoveManaged(adapterDir); err != nil {
		return false, err
	}
	return true, nil
}

// IsManagedLink reports whether linkPath is a symlink resolving to
// expectedTarget (i.e. it is "ours").
func IsManagedLink(linkPath, expectedTarget string) bool {
	if !dal.IsSymlink(linkPath) {
		return false
	}
	return dal.ResolveLink(linkPath) == expectedTarget
}

// StateLink classifies a symlink target by presence/resolution.
func StateLink(linkPath, expectedTarget string) common.InstallState {
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

// StateAdapter classifies a command's managed adapter directory.
func StateAdapter(adapterDir string, entry *common.Entry) common.InstallState {
	if dal.PathExists(adapterDir) {
		if dal.IsSymlink(adapterDir) {
			// A directory symlink is the legacy command-symlink shape. It
			// must not be treated as an adapter merely because it points at
			// the same entry; --force must be able to replace it.
			if dal.ResolveLink(adapterDir) == entry.Path {
				return common.InstallConflict
			}
			return common.InstallDangling
		}
		if IsManagedAdapter(adapterDir, entry) {
			return common.InstallInstalled
		}
		return common.InstallConflict
	}
	if dal.IsSymlink(adapterDir) {
		return common.InstallDangling
	}
	return common.InstallAbsent
}

// IsManagedAdapter reports whether dir is a managed adapter for entry: it
// carries the adapter marker and its regular SKILL.md contains the entry's
// marker (install-semantics.md).
func IsManagedAdapter(dir string, entry *common.Entry) bool {
	if !dal.PathExists(filepath.Join(dir, dal.AdapterMarker)) {
		return false
	}
	skillFile := filepath.Join(dir, "SKILL.md")
	if dal.IsSymlink(skillFile) {
		return false
	}
	actual, err := os.ReadFile(skillFile)
	if err != nil {
		return false
	}
	expected, err := os.ReadFile(entry.MarkerPath())
	return err == nil && string(actual) == string(expected)
}

// State derives the install health of a builtin-strategy install from the
// filesystem (FR-019). Unsupported strategies return InstallAbsent.
func State(strategy common.InstallStrategy, e *common.Entry, t common.InstallTarget) (common.InstallState, error) {
	switch strategy {
	case common.StrategySkillSymlink, common.StrategyCommandSymlink:
		return StateLink(filepath.Join(t.Path, installSlot(e)), e.Path), nil
	case common.StrategyCommandMarker:
		return StateLink(filepath.Join(t.Path, e.Name+".md"), e.MarkerPath()), nil
	case common.StrategyCommandAdapter:
		return StateAdapter(filepath.Join(t.Path, e.Name), e), nil
	}
	return common.InstallAbsent, fmt.Errorf("builtin strategy %q does not support state", strategy)
}

// RemoveForeign removes the non-managed object occupying the entry's slot
// (InstallConflict), restoring it to absent. Re-verifies the state so
// Uninstall's cases (managed install, dangling link) are never touched.
func RemoveForeign(strategy common.InstallStrategy, tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	var dest string
	var state common.InstallState
	switch strategy {
	case common.StrategySkillSymlink, common.StrategyCommandSymlink:
		dest = filepath.Join(t.Path, installSlot(e))
		state = StateLink(dest, e.Path)
	case common.StrategyCommandMarker:
		dest = filepath.Join(t.Path, e.Name+".md")
		state = StateLink(dest, e.MarkerPath())
	case common.StrategyCommandAdapter:
		dest = filepath.Join(t.Path, e.Name)
		state = StateAdapter(dest, e)
	default:
		return false, fmt.Errorf("builtin strategy %q does not support remove_foreign", strategy)
	}
	if state != common.InstallConflict {
		return false, nil
	}
	if err := tx.BackupRemove(dest); err != nil {
		return false, err
	}
	return true, nil
}

// InspectDangling enumerates target-side dangling symlinks for the builtin
// skill-symlink and command-marker strategies.
func InspectDangling(strategy common.InstallStrategy, t common.InstallTarget) ([]DanglingInstall, error) {
	if strategy != common.StrategySkillSymlink && strategy != common.StrategyCommandSymlink && strategy != common.StrategyCommandMarker {
		return nil, nil
	}
	children, err := os.ReadDir(t.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []DanglingInstall
	for _, child := range children {
		path := filepath.Join(t.Path, child.Name())
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}
		name := child.Name()
		if strategy == common.StrategyCommandMarker {
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			name = strings.TrimSuffix(name, ".md")
		}
		out = append(out, DanglingInstall{Name: name, Path: path, TargetName: t.Name, Strategy: strategy})
	}
	return out, nil
}

// RepairDangling removes one orphaned link only after rechecking that it is
// still a symlink and still unresolved. It never removes a real user file.
func RepairDangling(item DanglingInstall) error {
	fi, err := os.Lstat(item.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("fix: %q is no longer a dangling symlink; refusing to remove it", item.Path)
	}
	if _, err := os.Stat(item.Path); err == nil {
		return fmt.Errorf("fix: %q is no longer dangling; refusing to remove it", item.Path)
	}
	return os.Remove(item.Path)
}
