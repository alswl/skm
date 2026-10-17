package engines

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// ForkFinal switches a v3 Skill by replacing its complete upstream/fork
// directories under one recoverable transaction. The fork keeps only its
// immutable Git base; the upstream meta owns source identity and selection.
func (r *Repository) ForkFinal(e *common.Entry, opts SyncOptions) (*UpdateResult, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	meta, err := dal.ReadSkillMeta(upstream)
	if err != nil {
		return nil, err
	}
	if meta.SchemaVersion != 3 {
		return nil, fmt.Errorf("migration_required: skill uses legacy metadata")
	}
	rel, err := filepath.Rel(r.root, upstream)
	if err != nil || meta.Path != rel {
		return nil, fmt.Errorf("invalid skill metadata path")
	}
	if err = r.noFinalAttempt(rel); err != nil {
		return nil, err
	}
	if opts.Upstream && meta.Active == "upstream" {
		return &UpdateResult{Outcome: "current", ActivePath: upstream}, nil
	}
	forkRel := dal.SyncForkRel(rel)
	forkPath, err := dal.SyncPath(r.root, forkRel)
	if err != nil {
		return nil, err
	}
	oldFork, err := dal.TreeFingerprint(forkPath)
	if err != nil {
		return nil, err
	}
	reuseRetained := !opts.Upstream && oldFork != "absent" && meta.Active == "upstream" && !opts.Replace
	if !opts.Upstream && oldFork != "absent" && !opts.Replace && !reuseRetained {
		return nil, fmt.Errorf("existing_fork: %s; use fork --replace explicitly", forkPath)
	}
	result := &UpdateResult{Changed: true, Outcome: "switched", ActivePath: forkPath}
	if opts.Upstream {
		result.ActivePath = upstream
	}
	if opts.DryRun {
		return result, nil
	}
	tempRoot := filepath.Join(r.root, ".skm", "temp")
	if err = os.MkdirAll(tempRoot, 0700); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(tempRoot, "fork-")
	if err != nil {
		return result, err
	}
	defer func() {
		_ = os.RemoveAll(stage)
		_ = os.Remove(tempRoot)
		_ = os.Remove(filepath.Join(r.root, ".skm"))
	}()
	stagedUpstream := filepath.Join(stage, "upstream")
	if err = dal.CopySyncTree(upstream, stagedUpstream); err != nil {
		return result, err
	}
	if opts.Upstream {
		meta.Active = "upstream"
	} else {
		meta.Active = "fork"
	}
	if err = dal.WriteSkillMeta(stagedUpstream, *meta); err != nil {
		return result, err
	}
	upHash, err := dal.TreeFingerprint(upstream)
	if err != nil {
		return result, err
	}
	moves := []dal.SyncReplacement{{Path: rel, Source: stagedUpstream, Expected: upHash}}
	if !opts.Upstream && !reuseRetained {
		stagedFork := filepath.Join(stage, "fork")
		if err = dal.CopyPayloadSyncTree(upstream, stagedFork); err != nil {
			return result, err
		}
		forkMeta := common.ForkMeta{SchemaVersion: 3}
		if meta.RevisionKind == "git" {
			forkMeta.BaseRevision = meta.Revision
		}
		if err = dal.WriteForkMeta(stagedFork, forkMeta); err != nil {
			return result, err
		}
		moves = append(moves, dal.SyncReplacement{Path: forkRel, Source: stagedFork, Expected: oldFork})
	}
	if err = dal.CommitFinalSync(r.root, rel, moves); err != nil {
		result.Accepted = dal.IsSyncAccepted(err)
		return result, err
	}
	result.Accepted = true
	return result, nil
}

func (r *Repository) noFinalAttempt(rel string) error {
	path, _, err := r.finalAttempt(rel)
	if err != nil {
		return err
	}
	if path != "" {
		return fmt.Errorf("pending_attempt: inspect %s; use update --accept or --abort", path)
	}
	return nil
}
