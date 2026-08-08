package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// installAdapter creates a managed skill-form adapter directory for a command
// in a target whose declared strategy for command is command-adapter
// (FR-016): a SKILL.md link to the command marker plus links to auxiliary
// resources, so the command is discovered as a skill by that target's tool.
func (i *Installer) installAdapter(tx *dal.FileTransaction, entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	adapterDir := filepath.Join(target.Path, entry.Name)
	switch stateAdapter(adapterDir, entry) {
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
	if err := tx.CreateLink(entry.MarkerPath(), filepath.Join(adapterDir, "SKILL.md")); err != nil {
		return false, err
	}
	resources, err := entryResources(entry)
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

// entryResources lists the auxiliary files in an entry directory that must
// stay visible through the adapter (all files except the marker and
// meta.json). A single-file command (entry.Path is the .md marker itself, per
// Entry.MarkerPath) has no sibling directory to scan and so has no resources.
func entryResources(entry *common.Entry) ([]string, error) {
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
