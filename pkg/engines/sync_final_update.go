package engines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func (r *Repository) FinalForkMeta(e *common.Entry) (*common.ForkMeta, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	rel, err := filepath.Rel(r.root, upstream)
	if err != nil {
		return nil, err
	}
	return dal.ReadForkMeta(filepath.Join(r.root, dal.SyncForkRel(rel)))
}

// PrepareFinalUpdate works only from fixed trees supplied by services. No
// provider request or persistent baseline/cache write occurs in this layer.
func (r *Repository) PrepareFinalUpdate(ctx context.Context, e *common.Entry, meta *common.SkillMeta, base, fetched, version string, dryRun bool) (*UpdateResult, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	rel, err := filepath.Rel(r.root, upstream)
	if err != nil || rel != meta.Path {
		return nil, fmt.Errorf("invalid skill metadata path")
	}
	if err = r.noFinalAttempt(rel); err != nil {
		return nil, err
	}
	if _, err = os.Lstat(filepath.Join(fetched, "meta.json")); err == nil {
		return nil, fmt.Errorf("reserved upstream root meta.json")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	tempRoot := filepath.Join(r.root, ".skm", "temp")
	if err = os.MkdirAll(tempRoot, 0700); err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp(tempRoot, "update-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if work != "" {
			_ = os.RemoveAll(work)
		}
		_ = os.Remove(tempRoot)
		_ = os.Remove(filepath.Join(r.root, ".skm"))
	}()
	for _, item := range []struct{ src, name string }{{base, "base"}, {fetched, "raw"}} {
		if err = dal.CopyPayloadSyncTree(item.src, filepath.Join(work, item.name)); err != nil {
			return nil, err
		}
	}
	oldPayload, err := dal.PayloadSyncTree(filepath.Join(work, "base"))
	if err != nil {
		return nil, err
	}
	newPayload, err := dal.PayloadSyncTree(filepath.Join(work, "raw"))
	if err != nil {
		return nil, err
	}
	upHash, err := dal.TreeFingerprint(upstream)
	if err != nil {
		return nil, err
	}
	activeHash, err := dal.TreeFingerprint(e.Path)
	if err != nil {
		return nil, err
	}
	stagedUpstream := filepath.Join(work, "upstream")
	if err = dal.CopyPayloadSyncTree(filepath.Join(work, "raw"), stagedUpstream); err != nil {
		return nil, err
	}
	if err = normalizeStagedSkill(stagedUpstream, e.Name); err != nil {
		return nil, err
	}
	nextMeta := *meta
	nextMeta.Revision = version
	if err = dal.WriteSkillMeta(stagedUpstream, nextMeta); err != nil {
		return nil, err
	}
	result := &UpdateResult{Outcome: "updated", BaselineVersion: meta.Revision, UpstreamVersion: version}
	if meta.Active == "fork" {
		forkMeta, readErr := r.FinalForkMeta(e)
		if readErr != nil {
			return nil, readErr
		}
		result.BaselineVersion = forkMeta.BaseRevision
	}
	candidate := filepath.Join(work, "candidate")
	if meta.Active == "upstream" {
		oldNormalized := filepath.Join(work, "old-normalized")
		if err = dal.CopyPayloadSyncTree(filepath.Join(work, "base"), oldNormalized); err != nil {
			return nil, err
		}
		if err = normalizeStagedSkill(oldNormalized, e.Name); err != nil {
			return nil, err
		}
		want, err := dal.PayloadSyncTree(oldNormalized)
		if err != nil {
			return nil, err
		}
		got, err := dal.PayloadSyncTree(upstream)
		if err != nil {
			return nil, err
		}
		if want.Digest != got.Digest {
			return nil, fmt.Errorf("customization_requires_fork: community directory was edited; run skm fork %s before update", e.Name)
		}
		if err = dal.CopyPayloadSyncTree(stagedUpstream, candidate); err != nil {
			return nil, err
		}
	} else {
		if err = dal.CopyPayloadSyncTree(e.Path, filepath.Join(work, "local")); err != nil {
			return nil, err
		}
		patch, err := ApplyCustomization(ctx, filepath.Join(work, "base"), filepath.Join(work, "local"), filepath.Join(work, "raw"), candidate)
		if err != nil {
			return nil, err
		}
		result.Failures = patch.Failures
		if len(patch.Failures) > 0 {
			result.Outcome = "manual_required"
		}
	}
	if err = r.validateCandidate(candidate, e.Name); err != nil {
		result.Failures = append(result.Failures, SyncChange{Path: "SKILL.md", Kind: "invalid_candidate: " + err.Error()})
		result.Outcome = "manual_required"
	}
	operationID, err := newOperationID()
	if err != nil {
		return result, err
	}
	attempt := SyncAttempt{Schema: 3, OperationID: operationID, Entry: rel, RecordDigest: upHash, UpstreamDigest: upHash, LocalDigest: activeHash, BaseDigest: oldPayload.Digest, NewDigest: newPayload.Digest, Version: version, Failures: result.Failures}
	if err = dal.WriteSyncJSON(filepath.Join(work, "attempt.json"), attempt); err != nil {
		return result, err
	}
	if result.Outcome == "manual_required" {
		if dryRun {
			return result, fmt.Errorf("patch_failed: manual intervention required")
		}
		operations := filepath.Join(tempRoot, "operations")
		if err = os.MkdirAll(operations, 0700); err != nil {
			return result, err
		}
		path := filepath.Join(operations, operationID)
		if err = os.Rename(work, path); err != nil {
			return result, err
		}
		work = ""
		result.AttemptPath = path
		return result, fmt.Errorf("patch_failed: manual intervention required; see info %s", e.Name)
	}
	return r.acceptFinalAttempt(e, work, attempt, result, dryRun)
}

func (r *Repository) acceptFinalAttempt(e *common.Entry, work string, a SyncAttempt, result *UpdateResult, dryRun bool) (*UpdateResult, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	meta, err := dal.ReadSkillMeta(upstream)
	if err != nil {
		return result, err
	}
	if a.Schema != 3 || a.Entry != meta.Path || a.OperationID == "" {
		return result, fmt.Errorf("invalid update attempt")
	}
	upHash, err := dal.TreeFingerprint(upstream)
	if err != nil {
		return result, err
	}
	activeHash, err := dal.TreeFingerprint(e.Path)
	if err != nil {
		return result, err
	}
	if upHash != a.UpstreamDigest || activeHash != a.LocalDigest {
		return result, fmt.Errorf("stale_input: official content changed after update attempt")
	}
	for _, item := range []struct{ path, digest string }{{"base", a.BaseDigest}, {"raw", a.NewDigest}} {
		manifest, err := dal.PayloadSyncTree(filepath.Join(work, item.path))
		if err != nil || manifest.Digest != item.digest {
			return result, fmt.Errorf("corrupt update attempt: %s", item.path)
		}
	}
	candidate := filepath.Join(work, "candidate")
	for _, failure := range a.Failures {
		if failure.MarkerLabel == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(candidate, filepath.FromSlash(failure.Path)))
		if err == nil && strings.Contains(string(data), failure.MarkerLabel) {
			return result, fmt.Errorf("invalid_candidate: unresolved patch markers in %s", failure.Path)
		}
		if err != nil && !os.IsNotExist(err) {
			return result, err
		}
	}
	if err = r.validateCandidate(candidate, e.Name); err != nil {
		return result, err
	}
	stagedUpstream := filepath.Join(work, "upstream")
	nextMeta, err := dal.ReadSkillMeta(stagedUpstream)
	if err != nil || nextMeta.SchemaVersion != 3 || nextMeta.Revision != a.Version || nextMeta.Address != meta.Address || nextMeta.RepositoryAddress != meta.RepositoryAddress || nextMeta.RequestedRef != meta.RequestedRef || nextMeta.Subpath != meta.Subpath || nextMeta.Active != meta.Active {
		return result, fmt.Errorf("stale_input: prepared source identity changed")
	}
	ref := filepath.Join(work, "expected-upstream")
	if err = os.RemoveAll(ref); err != nil {
		return result, err
	}
	if err = dal.CopyPayloadSyncTree(filepath.Join(work, "raw"), ref); err != nil {
		return result, err
	}
	if err = normalizeStagedSkill(ref, e.Name); err != nil {
		return result, err
	}
	expectedUpstream, err := dal.PayloadSyncTree(ref)
	if err != nil {
		return result, err
	}
	actualUpstream, err := dal.PayloadSyncTree(stagedUpstream)
	if err != nil || actualUpstream.Digest != expectedUpstream.Digest {
		return result, fmt.Errorf("stale_input: prepared upstream changed")
	}
	moves := []dal.SyncReplacement{{Path: meta.Path, Source: stagedUpstream, Expected: upHash}}
	if meta.Active == "fork" {
		forkRel := dal.SyncForkRel(meta.Path)
		if _, err = os.Lstat(filepath.Join(candidate, "meta.json")); err == nil {
			return result, fmt.Errorf("invalid_candidate: reserved fork meta.json")
		} else if !os.IsNotExist(err) {
			return result, err
		}
		if err = dal.WriteForkMeta(candidate, common.ForkMeta{SchemaVersion: 3, BaseRevision: a.Version}); err != nil {
			return result, err
		}
		moves = append(moves, dal.SyncReplacement{Path: forkRel, Source: candidate, Expected: activeHash})
	}
	beforePayload, err := dal.PayloadSyncTree(e.Path)
	if err != nil {
		return result, err
	}
	afterPayload, err := dal.PayloadSyncTree(candidate)
	if err != nil {
		return result, err
	}
	if meta.Active == "upstream" {
		afterPayload, err = dal.PayloadSyncTree(stagedUpstream)
		if err != nil {
			return result, err
		}
	}
	result.Before, result.After = beforePayload.Digest, afterPayload.Digest
	result.Changed = meta.Revision != a.Version || result.Before != result.After
	if meta.Active == "fork" {
		forkMeta, readErr := r.FinalForkMeta(e)
		if readErr != nil {
			return result, readErr
		}
		result.Changed = result.Changed || forkMeta.BaseRevision != a.Version
	}
	result.ActivePath = e.Path
	if !result.Changed {
		result.Outcome = "current"
	} else {
		result.Outcome = "updated"
	}
	if dryRun || result.Outcome == "current" {
		return result, nil
	}
	if err = dal.CommitFinalSync(r.root, meta.Path, moves); err != nil {
		result.Accepted = dal.IsSyncAccepted(err)
		return result, err
	}
	result.Accepted = true
	return result, nil
}

func (r *Repository) FinishFinalUpdate(e *common.Entry, opts SyncOptions) (*UpdateResult, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	meta, err := dal.ReadSkillMeta(upstream)
	if err != nil {
		return nil, err
	}
	path, a, err := r.finalAttempt(meta.Path)
	if err != nil {
		return nil, err
	}
	result := &UpdateResult{Outcome: "cancelled", AttemptPath: path}
	if path == "" {
		return result, fmt.Errorf("rollback_unavailable: no unfinished update attempt")
	}
	if opts.Abort || opts.Rollback {
		current, err := dal.TreeFingerprint(e.Path)
		if err != nil {
			return result, err
		}
		if current != a.LocalDigest {
			return result, fmt.Errorf("stale_input: official content changed after update attempt")
		}
		result.Outcome = "rolled_back"
		if opts.DryRun {
			return result, nil
		}
		if err = os.RemoveAll(path); err != nil {
			return result, err
		}
		_ = os.Remove(filepath.Dir(path))
		_ = os.Remove(filepath.Dir(filepath.Dir(path)))
		_ = os.Remove(filepath.Join(r.root, ".skm"))
		return result, nil
	}
	result.Outcome = "updated"
	result, err = r.acceptFinalAttempt(e, path, a, result, opts.DryRun)
	if err == nil && !opts.DryRun {
		err = os.RemoveAll(path)
		_ = os.Remove(filepath.Dir(path))
		_ = os.Remove(filepath.Dir(filepath.Dir(path)))
		_ = os.Remove(filepath.Join(r.root, ".skm"))
	}
	return result, err
}

func (r *Repository) finalAttempt(rel string) (string, SyncAttempt, error) {
	operations := filepath.Join(r.root, ".skm", "temp", "operations")
	items, err := os.ReadDir(operations)
	if os.IsNotExist(err) {
		return "", SyncAttempt{}, nil
	}
	if err != nil {
		return "", SyncAttempt{}, err
	}
	var found string
	var attempt SyncAttempt
	for _, item := range items {
		if !item.IsDir() {
			return "", SyncAttempt{}, fmt.Errorf("recovery_required: invalid pending operation")
		}
		path := filepath.Join(operations, item.Name())
		var a SyncAttempt
		if err = dal.ReadSyncJSON(filepath.Join(path, "attempt.json"), &a); err != nil || a.Schema != 3 || a.OperationID != item.Name() {
			return "", SyncAttempt{}, fmt.Errorf("recovery_required: invalid pending operation %s", item.Name())
		}
		if a.Entry == rel {
			if found != "" {
				return "", SyncAttempt{}, fmt.Errorf("recovery_required: multiple pending operations for %s", rel)
			}
			found, attempt = path, a
		}
	}
	return found, attempt, nil
}
