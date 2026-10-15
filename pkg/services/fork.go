package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

func (s *Services) syncEntry(name string) (*common.Entry, error) {
	e, err := s.ResolveEntry(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, common.WithExitCode(fmt.Errorf("entry %q not found", name), common.ExitObject)
	}
	if e.Kind != common.KindSkill || e.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("sync requires an active skill: %s", name), common.ExitObject)
	}
	return e, nil
}
func (s *Services) Fork(ctx context.Context, name string, opts engines.SyncOptions) (*engines.UpdateResult, error) {
	return s.syncWrite(ctx, name, opts, func(e *common.Entry) (*engines.UpdateResult, error) { return s.Repo.ForkEntry(e, opts) })
}
func (s *Services) UseUpstream(ctx context.Context, name string, dryRun bool) (*engines.UpdateResult, error) {
	return s.Fork(ctx, name, engines.SyncOptions{Upstream: true, DryRun: dryRun})
}
func (s *Services) AcceptUpdate(ctx context.Context, name string, dryRun bool) (*engines.UpdateResult, error) {
	return s.finishUpdate(ctx, name, engines.SyncOptions{Accept: true, DryRun: dryRun})
}
func (s *Services) CancelUpdate(ctx context.Context, name string, dryRun bool) (*engines.UpdateResult, error) {
	return s.finishUpdate(ctx, name, engines.SyncOptions{Abort: true, DryRun: dryRun})
}
func (s *Services) RollbackUpdate(ctx context.Context, name string, dryRun bool) (*engines.UpdateResult, error) {
	return s.finishUpdate(ctx, name, engines.SyncOptions{Rollback: true, DryRun: dryRun})
}
func (s *Services) finishUpdate(ctx context.Context, name string, opts engines.SyncOptions) (*engines.UpdateResult, error) {
	return s.syncWrite(ctx, name, opts, func(e *common.Entry) (*engines.UpdateResult, error) { return s.Repo.FinishSyncUpdate(e, opts) })
}

func (s *Services) syncWrite(ctx context.Context, name string, opts engines.SyncOptions, action func(*common.Entry) (*engines.UpdateResult, error)) (*engines.UpdateResult, error) {
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
	e, err := s.syncEntry(name)
	if err != nil {
		return nil, err
	}
	beforeTargets := s.Installer.RefreshTargets(e)
	res, err := action(e)
	if res != nil {
		res.Entry = s.Repo.RelPath(e.Path)
		if e.RepositoryPath != "" {
			res.Entry = s.Repo.RelPath(e.RepositoryPath)
		}
		res.Preview = opts.DryRun
	}

	if err != nil {
		if res == nil {
			res = &engines.UpdateResult{Outcome: "blocked"}
		}
		if res.Accepted {
			if next, resolveErr := s.syncEntry(name); resolveErr == nil {
				if refreshErr := s.refreshSyncInstalls(next, res, beforeTargets...); refreshErr != nil {
					err = errors.Join(err, refreshErr)
				}
			}
		}
		res.Preview = opts.DryRun
		res.Reason = err.Error()
		res.ReasonCode = syncReasonCode(err)
		code := common.ExitCodeOf(err, common.ExitObject)
		var pathErr *os.PathError
		var executableErr *exec.Error
		var commandErr *exec.ExitError
		if (res.ReasonCode != "missing_baseline" && res.ReasonCode != "rollback_unavailable" && errors.As(err, &pathErr)) || errors.As(err, &executableErr) || errors.As(err, &commandErr) || ctx.Err() != nil {
			code = common.ExitError
		}
		if res.Entry == "" {
			res.Entry = s.Repo.RelPath(e.Path)
			if e.RepositoryPath != "" {
				res.Entry = s.Repo.RelPath(e.RepositoryPath)
			}
		}
		res.ReasonCode = syncReasonCode(err)
		res.NextActions = []string{"skm info " + name}
		if res.AttemptPath != "" {
			res.NextActions = append(res.NextActions, "skm update "+name+" --accept", "skm update "+name+" --abort")
		}
		return res, common.WithExitCode(err, code)
	}
	if opts.DryRun || !res.Accepted {
		return res, nil
	}
	next, err := s.syncEntry(name)
	if err != nil {
		return res, err
	}
	return res, s.refreshSyncInstalls(next, res, beforeTargets...)
}
func (s *Services) refreshSyncInstalls(e *common.Entry, result *engines.UpdateResult, previous ...common.InstallTarget) error {
	targets := s.Installer.RefreshTargets(e)
	seen := map[string]bool{}
	for _, t := range targets {
		seen[t.Name] = true
	}
	for _, t := range previous {
		if !seen[t.Name] {
			targets = append(targets, t)
			seen[t.Name] = true
		}
	}

	for _, t := range targets {
		seen[t.Name] = true
	}
	if e.AlternatePath != "" {
		alt := *e
		alt.Path = e.AlternatePath
		for _, t := range s.Installer.RefreshTargets(&alt) {
			if !seen[t.Name] {
				targets = append(targets, t)
				seen[t.Name] = true
			}
		}
	}
	var first error
	for _, t := range targets {
		tx := &dal.FileTransaction{}
		_, err := s.Installer.Install(tx, e, t, false)
		item := engines.RefreshResult{Target: t.Name, Success: err == nil}
		if err != nil {
			_ = tx.Rollback()
			item.Reason = err.Error()
			if first == nil {
				first = err
			}
		} else {
			tx.Commit()
		}
		result.Refresh = append(result.Refresh, item)
	}
	if first != nil {
		result.ReasonCode = "refresh_failed"
		result.NextActions = []string{"skm install " + e.Name}
		result.Reason = "refresh_failed: repository accepted; retry install: " + first.Error()
		return common.WithExitCode(fmt.Errorf("%s", result.Reason), common.ExitError)
	}
	return nil
}
func (s *Services) SyncStatus(name string) (*engines.SyncStatus, error) {
	lock, err := dal.AcquireLock(context.Background(), s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if err = s.Repo.SyncReadCheck(); err != nil {
		return nil, err
	}

	e, err := s.ResolveEntry(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("entry %q not found", name)
	}
	return s.Repo.SyncStatus(e)
}
func (s *Services) SyncRecover(ctx context.Context, dryRun bool) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	if dryRun {
		return s.Repo.SyncReadCheck()
	}
	return s.Repo.SyncRecover()
}
func (s *Services) updateSkill(ctx context.Context, name string, opts UpdateOptions) (*engines.UpdateResult, error) {
	return s.syncWrite(ctx, name, engines.SyncOptions{DryRun: opts.DryRun}, func(e *common.Entry) (*engines.UpdateResult, error) {
		if _, _, err := s.Repo.SyncRecord(e); err != nil {
			return nil, fmt.Errorf("missing_baseline: use sync baseline before update: %w", err)
		}
		st, err := s.Repo.SyncStatus(e)
		if err != nil {
			return nil, err
		}
		if st.AttemptPath != "" {
			return &engines.UpdateResult{Outcome: "manual_required", AttemptPath: st.AttemptPath}, fmt.Errorf("pending_attempt: use update --accept or --abort")
		}
		staged, cleanup, err := s.fetchFromOrigin(ctx, e)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		if _, err = os.Lstat(filepath.Join(staged, "meta.json")); err == nil {
			return nil, fmt.Errorf("reserved upstream root meta.json")
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		return s.Repo.PrepareSyncUpdate(ctx, e, staged, e.Origin.Revision, opts.DryRun)
	})
}

func syncReasonCode(err error) string {
	text := err.Error()
	for _, code := range []string{"customization_requires_fork", "missing_baseline", "corrupt_baseline", "existing_fork", "pending_attempt", "stale_input", "patch_failed", "unsupported_content", "invalid_candidate", "rollback_unavailable", "recovery_required", "refresh_failed"} {
		if strings.Contains(text, code+":") || text == code {
			return code
		}
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "io_failed"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "tool_failed"
	}
	return "blocked"
}
