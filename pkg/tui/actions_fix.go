package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// fixPreview is prepared off the event loop before showing the destructive
// confirmation. Diffing can walk an entire skill directory, so it must not be
// done from a key handler or View.
type fixPreview struct {
	name    string
	ref     string
	targets []string
	diff    string
}

type orphanDanglingPreview struct{ items []services.DanglingInstall }

// fixableTargets returns entry's per-target conflicts (a non-managed object
// already occupies the target) and dangling installs (a stray/broken link
// occupies it) — the two states fixSelected repairs. Both need the same
// repair: installSkill/installClaudeMarkdown/installAdapter (Force) already
// treat InstallConflict and InstallDangling identically (backup-remove
// whatever's there, then relink) — see pkg/services/skill_install.go. A plain
// Uninstall is *not* the fix for dangling: it only ever removes a link that
// currently resolves to entry's exact path (isManagedLink), so it silently
// no-ops on the common case — a stale link left behind after the entry moved,
// now resolving elsewhere.
//
// Read from the model's install state, never probed: this is called from two
// key handlers (F, and x to decide whether to offer the item at all), both on
// the event loop.
func (m *model) fixableTargets(entry *common.Entry) []string {
	return m.installs.broken(entry.Path)
}

// fixSelected repairs the current entry's conflict and dangling installs (key
// "F") by force-installing over each broken target — the same recovery
// already offered for a needs-force failure (see submitJobForce), just
// applied proactively instead of waiting for a rejected install. Scoped to
// the entry under the cursor only.
func (m *model) fixSelected() {
	if m.cursor >= len(m.filtered) {
		m.prepareOrphanDanglingFix()
		return
	}
	entry := m.filtered[m.cursor]
	if entry.Status != common.StatusActive {
		m.setStatus(fmt.Sprintf("%s is %s; nothing to fix", entry.Name, entry.Status))
		return
	}
	broken := m.fixableTargets(entry)
	if len(broken) == 0 {
		m.prepareOrphanDanglingFix()
		return
	}
	name := entry.Name
	ref := m.svc.Repo.RelPath(entry.Path)
	m.submitJob("prepare fix "+name, func(ctx context.Context) (any, error) {
		return m.prepareFixPreview(ctx, entry, ref, broken), nil
	})
}

// prepareOrphanDanglingFix handles links whose original source disappeared,
// so they cannot be reached from an Entry selection. It is intentionally a
// background scan: target directories and plugin inspectors may be slow.
func (m *model) prepareOrphanDanglingFix() {
	m.submitJob("inspect orphan dangling installs", func(ctx context.Context) (any, error) {
		items, err := m.svc.OrphanDangling(ctx)
		if err != nil {
			return nil, err
		}
		return orphanDanglingPreview{items: items}, nil
	})
}

func (m *model) showOrphanDanglingConfirmation(preview orphanDanglingPreview) {
	if len(preview.items) == 0 {
		m.setStatus("fix: no conflicts or dangling installs")
		return
	}
	labels := make([]string, len(preview.items))
	for i, item := range preview.items {
		labels[i] = fmt.Sprintf("%s (%s)", item.Name, item.TargetName)
	}
	m.confirm = &pages.Confirm{Prompt: fmt.Sprintf("Fix %d orphan dangling install(s)? This removes only invalid links: %s", len(preview.items), strings.Join(labels, ", ")), OnYes: func() {
		m.submitJob(fmt.Sprintf("fix %d orphan dangling install(s)", len(preview.items)), func(ctx context.Context) (any, error) {
			for _, item := range preview.items {
				if err := m.svc.CleanDangling(ctx, item); err != nil {
					return nil, err
				}
			}
			return fmt.Sprintf("fixed %d orphan dangling install(s)", len(preview.items)), nil
		})
	}}
}

// prepareFixPreview collects a readable content diff for real target-side
// conflicts. Dangling links have no content to compare, but are still listed
// as replacements in the following confirmation.
func (m *model) prepareFixPreview(ctx context.Context, entry *common.Entry, ref string, targets []string) fixPreview {
	var parts []string
	for _, targetName := range targets {
		if m.installState(entry.Path, targetName) != common.InstallConflict {
			continue
		}
		target, ok := m.svc.Installer.TargetByName(targetName)
		if !ok {
			continue
		}
		diff, err := m.svc.Installer.Diff(ctx, entry, target)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s: target plugin did not provide a diff: %v", targetName, err))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:\n%s", targetName, diff))
	}
	return fixPreview{name: entry.Name, ref: ref, targets: targets, diff: strings.TrimSpace(strings.Join(parts, "\n\n"))}
}

func (m *model) installState(path, target string) common.InstallState {
	for _, cell := range m.installs.forEntry(path) {
		if cell.name == target {
			return cell.state
		}
	}
	return common.InstallAbsent
}

// showFixConfirmation is called only after prepareFixPreview completed, so
// the confirmation can offer a diff without blocking terminal input.
func (m *model) showFixConfirmation(preview fixPreview) {
	prompt := fmt.Sprintf("Fix %s? This will replace with managed installs: %s", preview.name, strings.Join(preview.targets, ", "))
	if preview.diff != "" {
		prompt += ". Review the diff before applying."
	}
	name, ref, broken := preview.name, preview.ref, preview.targets
	m.confirm = &pages.Confirm{Prompt: prompt, Diff: preview.diff, OnYes: func() {
		m.submitJob("fix "+name, func(ctx context.Context) (any, error) {
			result, err := m.svc.Install(ctx, ref, services.InstallOptions{Targets: broken, Force: true})
			if err != nil {
				return nil, err
			}
			return installStatusMessage("fix", name, result), nil
		})
	}}
}
