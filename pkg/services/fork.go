package services

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

// ForkOptions selects what Fork does. With neither flag it creates a fork.
type ForkOptions struct {
	Replace bool // discard the existing fork and copy the upstream again
	Discard bool // delete the fork and go back to the upstream
}

// ForkResult is the CLI JSON report for fork.
type ForkResult struct {
	Name         string `json:"name"`
	Action       string `json:"action"` // forked | replaced | discarded
	ActivePath   string `json:"active_path"`
	UpstreamPath string `json:"upstream_path"`
}

// Fork gives a Skill its own editable copy in skills-fork/, replaces that copy,
// or deletes it. The fork's presence is the whole state: while it exists it is
// the active content. Installs are moved along with it in one transaction.
func (s *Services) Fork(ctx context.Context, name string, opts ForkOptions) (*ForkResult, error) {
	if opts.Replace && opts.Discard {
		return nil, common.WithExitCode(fmt.Errorf("fork: --replace and --discard cannot be combined"), common.ExitObject)
	}
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("fork: %w", err)
	}
	if entry == nil {
		return nil, common.WithExitCode(fmt.Errorf("fork: entry %q not found", name), common.ExitObject)
	}
	if entry.Kind != common.KindSkill || entry.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("fork: %q must be an active skill", name), common.ExitObject)
	}
	forked := entry.UpstreamPath != ""
	upstream := engines.UpstreamOf(entry)
	forkPath, err := s.Repo.ForkPath(upstream)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("fork: %w", err), common.ExitObject)
	}
	switch {
	case opts.Discard && !forked:
		return nil, common.WithExitCode(fmt.Errorf("fork: %q has no fork", name), common.ExitObject)
	case !opts.Discard && !opts.Replace && forked:
		return nil, common.WithExitCode(fmt.Errorf("fork: %q already has a fork; use --replace to copy the upstream again or --discard to remove it", name), common.ExitObject)
	}

	action := "forked"
	var stage string
	switch {
	case opts.Discard:
		action = "discarded"
	default:
		if opts.Replace && forked {
			action = "replaced"
		}
		if stage, err = s.Repo.StageFork(upstream); err != nil {
			return nil, common.WithExitCode(fmt.Errorf("fork: %w", err), common.ExitError)
		}
	}

	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	// Reinstall rather than patch links in place: the installed path changes
	// between upstream and fork, so each target is uninstalled from the old
	// path and installed again from the new one.
	targets := s.Installer.RefreshTargets(entry)
	tx := &dal.FileTransaction{}
	fail := func(err error) (*ForkResult, error) {
		_ = tx.Rollback()
		return nil, common.WithExitCode(fmt.Errorf("fork: %w", err), common.ExitError)
	}
	for _, t := range targets {
		if _, err := s.Installer.Uninstall(tx, entry, t); err != nil {
			return fail(err)
		}
	}
	if opts.Discard {
		err = tx.BackupRemove(forkPath)
	} else {
		err = tx.MoveStage(stage, forkPath)
	}
	if err != nil {
		return fail(err)
	}
	after, err := s.ResolveEntry(s.Repo.RelPath(upstream))
	if err != nil || after == nil {
		return fail(fmt.Errorf("cannot find the entry after the change"))
	}
	for _, t := range targets {
		if _, err := s.Installer.Install(tx, after, t, false); err != nil {
			return fail(err)
		}
	}
	tx.Commit()
	res := &ForkResult{Name: after.Name, Action: action, ActivePath: after.Path, UpstreamPath: upstream}
	return res, nil
}

// updateFork carries the fork's changes onto freshly fetched upstream content
// and replaces both directories, or changes nothing at all.
func (s *Services) updateFork(ctx context.Context, entry *common.Entry, staged string, opts UpdateOptions) (*engines.UpdateResult, error) {
	kind, name, err := s.Repo.ProbeStaged(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if kind != entry.Kind || name != entry.Name {
		return nil, common.WithExitCode(fmt.Errorf("update: fetched copy is %s %q, expected %s %q", kind, name, entry.Kind, entry.Name), common.ExitObject)
	}
	merge, err := s.Repo.MergeFork(ctx, entry.UpstreamPath, entry.Path, staged, engines.ForkMergeTimeout)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("update: %q was not changed: %w", entry.Name, err), common.ExitObject)
	}
	defer merge.Cleanup()
	res := &engines.UpdateResult{Before: merge.Before, After: merge.After, Changed: merge.Changed}
	if !merge.Changed || opts.DryRun {
		return res, nil
	}
	if err := s.Repo.CommitForkUpdate(ctx, entry, merge); err != nil {
		return nil, err
	}
	if err := s.refreshInstalls(ctx, entry); err != nil {
		return nil, fmt.Errorf("update: refresh installs: %w", err)
	}
	return res, nil
}
