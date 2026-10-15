package services

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

func (s *Services) trackedLifecycle(ctx context.Context, name, action, provider string, kind common.EntryKind, opts engines.LifecycleOptions, uninstall bool) (*LifecycleResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if opts.DryRun {
		err = s.Repo.SyncReadCheck()
	} else {
		err = s.Repo.SyncRecover()
	}
	if err != nil {
		return nil, err
	}
	e, err := s.ResolveEntry(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, notFound(action, name)
	}
	// Preflight happens before any target mutation, under this same lock.
	dest, err := s.Repo.SyncLifecycle(e, action, provider, kind, engines.LifecycleOptions{Force: opts.Force, DryRun: true})
	if err != nil {
		return nil, err
	}
	res := &LifecycleResult{Action: action, Name: name, Path: dest, DryRun: opts.DryRun, Success: true}
	if (action == "delete" || action == "convert") && e.AlternatePath != "" {
		res.Warnings = []string{"Retained fork preserved at " + e.AlternatePath + "; it will not receive updates."}
	}

	if opts.DryRun {
		return res, nil
	}
	targets := s.Installer.RefreshTargets(e)
	tx := &dal.FileTransaction{}
	if uninstall || action == "normalize" || action == "convert" {
		for _, target := range targets {
			if _, err = s.Installer.Uninstall(tx, e, target); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
		}
	}
	dest, err = s.Repo.SyncLifecycle(e, action, provider, kind, opts)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	tx.Commit()
	res.Path = dest
	if action == "normalize" || action == "convert" {
		next, err := s.ResolveEntry(dest)
		if err != nil {
			return res, err
		}
		if next == nil {
			return res, fmt.Errorf("moved entry not found")
		}
		for _, target := range targets {
			tx := &dal.FileTransaction{}
			if _, err = s.Installer.Install(tx, next, target, false); err != nil {
				_ = tx.Rollback()
				res.Success = false
				return res, err
			}
			tx.Commit()
		}
	}
	return res, nil
}
func (s *Services) ArchiveWithUninstall(ctx context.Context, name string, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	e, err := s.ResolveEntry(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, notFound("archive", name)
	}
	if e.RepositoryPath != "" {
		return s.trackedLifecycle(ctx, name, "archive", "", "", opts, true)
	}
	// Legacy entries cannot carry a sync scene; retain their established behavior.
	if !opts.DryRun {
		if _, err = s.Uninstall(ctx, name, InstallOptions{}); err != nil {
			return nil, err
		}
	}
	return s.Archive(ctx, name, opts)
}
