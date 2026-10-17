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
	// Scanning deliberately marks entries behind an interrupted transaction as
	// errors. Rollback still needs their stable upstream path to locate the
	// journal, so permit that narrow recovery-only representation here.
	if e.Kind == common.KindSkill && e.Status == common.StatusError && e.Error != nil && strings.Contains(*e.Error, "recovery_required:") {
		e.Status = common.StatusActive
	}
	if e.Kind != common.KindSkill || e.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("sync requires an active skill: %s", name), common.ExitObject)
	}
	return e, nil
}
func (s *Services) Fork(ctx context.Context, name string, opts engines.SyncOptions) (*engines.UpdateResult, error) {
	return s.syncWrite(ctx, name, opts, func(e *common.Entry) (*engines.UpdateResult, error) {
		upstream := e.RepositoryPath
		if upstream == "" {
			upstream = e.Path
		}
		if meta, err := dal.ReadSkillMeta(upstream); err == nil && meta.SchemaVersion == 3 {
			return s.Repo.ForkFinal(e, opts)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if opts.Upstream {
			return s.Repo.ForkEntry(e, opts)
		}
		rec, rel, err := s.Repo.SyncRecord(e)
		if err != nil {
			return nil, err
		}
		base, cleanup, err := s.resolveBaselineTree(ctx, rec, rec.UpstreamBase, filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), "upstream-baseline"))
		if err != nil {
			return nil, err
		}
		defer cleanup()
		return s.Repo.ForkEntryFromBase(e, base, opts)
	})
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
	return s.syncWrite(ctx, name, opts, func(e *common.Entry) (*engines.UpdateResult, error) {
		upstream := e.RepositoryPath
		if upstream == "" {
			upstream = e.Path
		}
		if meta, err := dal.ReadSkillMeta(upstream); err == nil && meta.SchemaVersion == 3 {
			return s.Repo.FinishFinalUpdate(e, opts)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return s.Repo.FinishSyncUpdate(e, opts)
	})
}

func (s *Services) syncWrite(ctx context.Context, name string, opts engines.SyncOptions, action func(*common.Entry) (*engines.UpdateResult, error)) (*engines.UpdateResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if opts.Rollback || opts.Abort {
		// A rollback is the user's explicit recovery choice. Do not run the
		// normal auto-recovery gate, which intentionally refuses to choose it.
		err = s.Repo.SyncReadCheck()
		if err != nil && strings.Contains(err.Error(), "recovery_required:") {
			if result, found, rollbackErr := s.rollbackMissingSyncEntry(name, opts.DryRun); found || rollbackErr != nil {
				return result, rollbackErr
			}
			err = nil
		}
	} else if opts.DryRun {
		err = s.Repo.SyncReadCheck()
	} else {
		err = s.Repo.SyncRecover()
	}
	if err != nil {
		return nil, err
	}
	e, err := s.syncEntry(name)
	if err != nil {
		if opts.Rollback || opts.Abort {
			if result, found, rollbackErr := s.rollbackMissingSyncEntry(name, opts.DryRun); found || rollbackErr != nil {
				return result, rollbackErr
			}
		}
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
		if res.ReasonCode == "missing_baseline" {
			res.NextActions = append(res.NextActions, "skm sync baseline "+name+" --help")
		}
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

func (s *Services) rollbackMissingSyncEntry(name string, dryRun bool) (*engines.UpdateResult, bool, error) {
	entries, err := dal.InterruptedSyncEntries(s.Cfg.Root)
	if err != nil {
		return nil, false, err
	}
	var match string
	for _, entry := range entries {
		if entry != name && filepath.Base(entry) != name {
			continue
		}
		if match != "" {
			return nil, true, fmt.Errorf("recovery_blocked: multiple interrupted entries match %q", name)
		}
		match = entry
	}
	if match == "" {
		return nil, false, nil
	}
	result := &engines.UpdateResult{Entry: match, Outcome: "rolled_back", Preview: dryRun}
	if err := dal.RollbackSync(s.Cfg.Root, match, dryRun); err != nil {
		result.Reason, result.ReasonCode = err.Error(), syncReasonCode(err)
		return result, true, err
	}
	return result, true, nil
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
	_, err := s.SyncRecoverReport(ctx, dryRun)
	return err
}

type SyncRecoveryOperation struct {
	ID, Entry, State, Outcome, Reason string
}

type SyncRecoveryResult struct {
	Success    bool                    `json:"success"`
	Operations []SyncRecoveryOperation `json:"operations"`
}

func (s *Services) SyncRecoverReport(ctx context.Context, dryRun bool) (*SyncRecoveryResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	transactions, err := dal.SyncTransactions(s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	result := &SyncRecoveryResult{Operations: make([]SyncRecoveryOperation, 0, len(transactions))}
	if dryRun {
		err = s.Repo.SyncReadCheck()
	} else {
		err = s.Repo.SyncRecover()
	}
	for _, transaction := range transactions {
		operation := SyncRecoveryOperation{ID: transaction.ID, Entry: transaction.Entry, State: transaction.State, Outcome: "recovery_required"}
		switch transaction.State {
		case "rolling_back":
			operation.Outcome = "rolled_back"
		case "committed", "rolled_back":
			operation.Outcome = "cleaned"
		}
		if err != nil {
			operation.Outcome, operation.Reason = "blocked", err.Error()
		}
		result.Operations = append(result.Operations, operation)
	}
	result.Success = err == nil
	return result, err
}
func (s *Services) updateSkill(ctx context.Context, name string, opts UpdateOptions) (*engines.UpdateResult, error) {
	return s.syncWrite(ctx, name, engines.SyncOptions{DryRun: opts.DryRun}, func(e *common.Entry) (*engines.UpdateResult, error) {
		upstream := e.RepositoryPath
		if upstream == "" {
			upstream = e.Path
		}
		if meta, metaErr := dal.ReadSkillMeta(upstream); metaErr == nil && meta.SchemaVersion == 3 {
			return s.updateFinalSkill(ctx, e, meta, opts)
		} else if metaErr != nil && !os.IsNotExist(metaErr) {
			return nil, metaErr
		}
		if _, _, err := s.Repo.SyncRecord(e); err != nil {
			if !os.IsNotExist(err) || e.Origin == nil {
				return nil, s.syncRecordError(name, err)
			}
			match, matchErr := s.matchLegacyBaseline(ctx, e)
			if matchErr != nil {
				return nil, matchErr
			}
			if match == nil {
				return nil, s.syncRecordError(name, err)
			}
			defer match.Cleanup()
			fetched, fetchErr := s.fetchFromOrigin(ctx, e, !opts.DryRun)
			if fetchErr != nil {
				return nil, fetchErr
			}
			defer fetched.Cleanup()
			origin := match.Origin
			origin.Revision = fetched.Version
			return s.Repo.UpgradeLegacyImport(e, match.Path, fetched.Path, &origin, opts.DryRun)
		}
		st, err := s.Repo.SyncStatus(e)
		if err != nil {
			return nil, err
		}
		if st.AttemptPath != "" {
			return &engines.UpdateResult{Outcome: "manual_required", AttemptPath: st.AttemptPath}, fmt.Errorf("pending_attempt: use update --accept or --abort")
		}
		fetched, err := s.fetchFromOrigin(ctx, e, !opts.DryRun)
		if err != nil {
			return nil, err
		}
		defer fetched.Cleanup()
		if _, err = os.Lstat(filepath.Join(fetched.Path, "meta.json")); err == nil {
			return nil, fmt.Errorf("reserved upstream root meta.json")
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		rec, rel, err := s.Repo.SyncRecord(e)
		if err != nil {
			return nil, err
		}
		kind, ref := "upstream-baseline", rec.UpstreamBase
		if rec.Active == "fork" {
			kind = "fork-baseline"
			if rec.ForkBase != nil {
				ref = *rec.ForkBase
			}
		}
		base, release, err := s.resolveBaselineTree(ctx, rec, ref, filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), kind))
		if err != nil {
			return nil, err
		}
		defer release()
		return s.Repo.PrepareSyncUpdateFromBase(ctx, e, base, fetched.Path, fetched.Version, opts.DryRun)
	})
}

func (s *Services) syncRecordError(name string, err error) error {
	if !os.IsNotExist(err) {
		return err
	}
	stateRoot := filepath.Join(s.Cfg.Root, ".skm")
	if _, statErr := os.Lstat(stateRoot); statErr == nil {
		if _, formatErr := os.Lstat(filepath.Join(stateRoot, "format.json")); formatErr != nil {
			return fmt.Errorf("unrecognized sync root %s: %w", stateRoot, formatErr)
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	return fmt.Errorf("missing_baseline: %q has no saved upstream baseline; run skm sync baseline %s --help", name, name)
}

func syncReasonCode(err error) string {
	text := err.Error()
	for _, code := range []string{"not_applicable", "historical_source_unavailable", "customization_requires_fork", "missing_baseline", "corrupt_baseline", "existing_fork", "pending_attempt", "stale_input", "patch_failed", "unsupported_content", "invalid_candidate", "rollback_unavailable", "recovery_required", "recovery_blocked", "migration_required", "migration_conflict", "refresh_failed"} {
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
