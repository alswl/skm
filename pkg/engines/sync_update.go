package engines

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func (r *Repository) SyncStatus(e *common.Entry) (*SyncStatus, error) {
	st := &SyncStatus{AttemptState: "none", Mode: "upstream", ForkState: "absent", ActivePath: e.Path, UpstreamPath: e.Path, BaselineState: "missing", Customization: "unknown", Origin: e.Origin, Changes: []SyncChange{}}
	rec, rel, err := r.SyncRecord(e)
	if os.IsNotExist(err) {
		if e.Origin == nil {
			st.BaselineState = "not_applicable"
			st.Customization = "not_applicable"
		}
		return st, nil
	}
	if err != nil {
		st.BaselineState = "corrupt"
		return st, err
	}
	st.Mode = rec.Active
	st.UpstreamPath = filepath.Join(r.root, rel)
	st.Origin = &rec.Origin
	baseName := "upstream-baseline"
	digest := rec.UpstreamDigest
	st.BaselineVersion = rec.UpstreamVersion
	if rec.ForkPath != "" {
		st.ForkPath = filepath.Join(r.root, rec.ForkPath)
		st.ForkState = "retained"
	}
	if rec.Active == "fork" {
		baseName = "fork-baseline"
		digest = rec.ForkDigest
		st.BaselineVersion = rec.ForkVersion
		st.ForkState = "active"
	}
	base := filepath.Join(r.root, dal.SyncStateRel(rel), baseName)
	ref := rec.UpstreamBase
	if rec.Active == "fork" && rec.ForkBase != nil {
		ref = *rec.ForkBase
	}
	if ref.Kind == "git" {
		repository := rec.Origin.RepositoryAddress
		if repository == "" {
			repository = rec.Origin.Address
		}
		cached, cacheErr := dal.ReadGitCache(r.root, repository, ref.Revision)
		if cacheErr != nil {
			// Catalog, info and install must remain offline. A Git baseline is
			// reconstructible, so a missing cache is actionable rather than a
			// corrupt record or a reason to silently contact the source.
			st.BaselineState = "fetch_required"
			st.NextActions = append(st.NextActions, "run sync update or sync diff when the source is available")
			return st, nil
		}
		base, cacheErr = cachedSyncSubpath(cached, rec.Origin.Subpath)
		if cacheErr != nil {
			st.BaselineState = "fetch_required"
			st.NextActions = append(st.NextActions, "refresh the Git cache from the recorded source")
			return st, nil
		}
	}
	if ref.Kind == "snapshot" && ref.SnapshotDigest != "" {
		if snapshotRel, pathErr := dal.SyncBaselineRel(ref.SnapshotDigest); pathErr == nil {
			candidate := filepath.Join(r.root, snapshotRel)
			if m, treeErr := dal.SyncTree(candidate); treeErr == nil && m.Digest == digest {
				base = candidate
			}
		}
	}
	bm, err := dal.SyncTree(base)
	if err != nil || bm.Digest != digest {
		st.BaselineState = "corrupt"
		return st, fmt.Errorf("corrupt_baseline: %s", base)
	}
	lm, err := dal.SyncTree(e.Path)
	if err != nil {
		return st, err
	}
	st.BaselineState = "valid"
	st.Customization = "clean"
	if bm.Digest != lm.Digest {
		st.Customization = "customized"
	}
	nodes := map[string]dal.SyncNode{}
	for _, n := range bm.Nodes {
		nodes[n.Path] = n
	}
	for _, n := range lm.Nodes {
		b, ok := nodes[n.Path]
		if !ok {
			st.Changes = append(st.Changes, SyncChange{Path: n.Path, Kind: "added"})
		} else if n != b {
			st.Changes = append(st.Changes, SyncChange{Path: n.Path, Kind: "modified"})
		}
		delete(nodes, n.Path)
	}
	for path := range nodes {
		st.Changes = append(st.Changes, SyncChange{Path: path, Kind: "deleted"})
	}
	slices.SortFunc(st.Changes, func(a, b SyncChange) int { return strings.Compare(a.Path, b.Path) })
	pending, attempt, err := r.operationAttempt(rel)
	if err != nil {
		return st, err
	}
	if pending != "" {
		st.AttemptPath = pending
		if attempt.Schema != 2 || attempt.Entry != rel || attempt.OperationID == "" {
			st.AttemptState = "corrupt"
			st.OperationState = "blocked"
			st.RecoveryRequired = true
		} else {
			st.AttemptState = "failed"
			st.AttemptVersion = attempt.Version
			st.Failures = attempt.Failures
			st.OperationID = attempt.OperationID
			st.OperationState = "failed_candidate"
		}
		st.NextActions = []string{"skm update " + e.Name + " --accept", "skm update " + e.Name + " --abort"}

	}
	// Rollback is intentionally limited to the current unfinished candidate.
	// A successfully accepted update leaves no history snapshot behind.
	st.RollbackAvailable = st.AttemptPath != ""
	return st, nil
}

func cachedSyncSubpath(root, subpath string) (string, error) {
	if subpath == "" || subpath == "." {
		return root, nil
	}
	if !filepath.IsLocal(subpath) {
		return "", fmt.Errorf("unsafe source subpath")
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, subpath))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("source subpath escapes cache")
	}
	return path, nil
}

func (r *Repository) noAttempt(rel string) error {
	path, _, err := r.operationAttempt(rel)
	if err != nil {
		return err
	}
	if path != "" {
		return fmt.Errorf("pending_attempt: inspect %s; use update --accept or --abort", path)
	}
	return nil
}

// operationAttempt finds one operation by its immutable logical entry. The
// old pending path is read only for an upgrade-safe cancellation path.
func (r *Repository) operationAttempt(rel string) (string, SyncAttempt, error) {
	operations, err := dal.SyncPath(r.root, filepath.Join(".skm", "work", "operations"))
	if err != nil {
		return "", SyncAttempt{}, err
	}
	dirs, err := os.ReadDir(operations)
	if err == nil {
		var found string
		var attempt SyncAttempt
		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}
			var candidate SyncAttempt
			if err := dal.ReadSyncJSON(filepath.Join(operations, dir.Name(), "attempt.json"), &candidate); err != nil {
				return "", SyncAttempt{}, fmt.Errorf("recovery_required: invalid operation %s", dir.Name())
			}
			if candidate.Entry != rel {
				continue
			}
			if found != "" {
				return "", SyncAttempt{}, fmt.Errorf("recovery_required: multiple operations for %s", rel)
			}
			found, attempt = filepath.Join(operations, dir.Name()), candidate
		}
		if found != "" {
			return found, attempt, nil
		}
	} else if !os.IsNotExist(err) {
		return "", SyncAttempt{}, err
	}
	legacy := filepath.Join(r.root, dal.SyncLocalRel("pending", rel))
	var attempt SyncAttempt
	if err := dal.ReadSyncJSON(filepath.Join(legacy, "attempt.json"), &attempt); err == nil {
		return legacy, attempt, nil
	} else if !os.IsNotExist(err) {
		return "", SyncAttempt{}, err
	}
	return "", SyncAttempt{}, nil
}

func newOperationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
func (r *Repository) recordStage(rel string) (string, string, error) {
	path := filepath.Join(r.root, dal.SyncStateRel(rel))
	hash, err := dal.TreeFingerprint(path)
	if err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp("", "skm-record-*")
	if err != nil {
		return "", "", err
	}
	if err = dal.CopySyncTree(path, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", "", err
	}
	return tmp, hash, nil
}
func replaceTree(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return dal.CopySyncTree(src, dst)
}

// ForkEntry and the update methods require the caller's repository lock.
func (r *Repository) ForkEntry(e *common.Entry, opts SyncOptions) (*UpdateResult, error) {
	return r.forkEntry(e, "", opts)
}

// ForkEntryFromBase accepts the services-resolved immutable upstream raw tree.
// New v2 records store snapshots centrally and Git bases in the shared cache,
// so migration must not depend on a retired entry-sidecar baseline.
func (r *Repository) ForkEntryFromBase(e *common.Entry, base string, opts SyncOptions) (*UpdateResult, error) {
	return r.forkEntry(e, base, opts)
}

func (r *Repository) forkEntry(e *common.Entry, resolvedBase string, opts SyncOptions) (*UpdateResult, error) {
	rec, rel, err := r.SyncRecord(e)
	if err != nil {
		return nil, fmt.Errorf("missing_baseline: restore a baseline before forking: %w", err)
	}
	if err = r.noAttempt(rel); err != nil {
		return nil, err
	}
	if opts.Upstream && rec.Active == "upstream" {
		return &UpdateResult{Outcome: "current"}, nil
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
	if !opts.Upstream && oldFork != "absent" && !opts.Replace {
		return nil, fmt.Errorf("existing_fork: %s; handle it manually or explicitly use fork --replace", forkPath)
	}
	state, oldState, err := r.recordStage(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(state) }()
	replacements := []dal.SyncReplacement{}
	if opts.Upstream {
		rec.Active = "upstream"
	} else {
		base := resolvedBase
		if base == "" {
			base = filepath.Join(state, "upstream-baseline")
		}
		m, err := dal.SyncTree(base)
		if err != nil {
			return nil, err
		}
		if m.Digest != rec.UpstreamDigest {
			return nil, fmt.Errorf("corrupt_baseline")
		}
		if err = replaceTree(base, filepath.Join(state, "fork-baseline")); err != nil {
			return nil, err
		}
		rec.ForkPath = forkRel
		rec.ForkVersion = rec.UpstreamVersion
		rec.ForkDigest = rec.UpstreamDigest
		forkBase := rec.UpstreamBase
		rec.ForkBase = &forkBase
		rec.Active = "fork"
		replacements = append(replacements, dal.SyncReplacement{Path: forkRel, Source: filepath.Join(r.root, rel), Expected: oldFork})
	}
	rec.Generation++
	if err = dal.WriteSyncJSON(filepath.Join(state, "record.json"), rec); err != nil {
		return nil, err
	}
	replacements = append(replacements, dal.SyncReplacement{Path: dal.SyncStateRel(rel), Source: state, Expected: oldState})
	result := &UpdateResult{Changed: true, Outcome: "switched", ActivePath: filepath.Join(r.root, rel)}
	if rec.Active == "fork" {
		result.ActivePath = forkPath
	}
	if opts.DryRun {
		return result, nil
	}
	if err = dal.CommitSync(r.root, rel, replacements); err != nil {
		result.Accepted = dal.IsSyncAccepted(err)
		return result, err
	}
	result.Accepted = true
	// Switching changes the record fingerprint, so any old rollback is invalid.
	return result, nil
}

func (r *Repository) PrepareSyncUpdate(ctx context.Context, e *common.Entry, upstream, version string, dryRun bool) (*UpdateResult, error) {
	return r.prepareSyncUpdate(ctx, e, "", upstream, version, dryRun)
}

// PrepareSyncUpdateFromBase accepts a services-resolved immutable raw B.
// Engines intentionally do not fetch: network/cache ownership stays above
// the domain layer and candidates are built only from fixed inputs.
func (r *Repository) PrepareSyncUpdateFromBase(ctx context.Context, e *common.Entry, base, upstream, version string, dryRun bool) (*UpdateResult, error) {
	return r.prepareSyncUpdate(ctx, e, base, upstream, version, dryRun)
}

func (r *Repository) prepareSyncUpdate(ctx context.Context, e *common.Entry, resolvedBase, upstream, version string, dryRun bool) (*UpdateResult, error) {
	rec, rel, err := r.SyncRecord(e)
	if err != nil {
		return nil, fmt.Errorf("missing_baseline: use sync baseline: %w", err)
	}
	if err = r.noAttempt(rel); err != nil {
		path, _, _ := r.operationAttempt(rel)
		return &UpdateResult{Outcome: "manual_required", AttemptPath: path}, err
	}
	state := filepath.Join(r.root, dal.SyncStateRel(rel))
	baseName := "upstream-baseline"
	baseDigest := rec.UpstreamDigest
	if rec.Active == "fork" {
		baseName = "fork-baseline"
		baseDigest = rec.ForkDigest
	}
	recordHash, err := dal.TreeFingerprint(state)
	if err != nil {
		return nil, err
	}
	upHash, err := dal.TreeFingerprint(filepath.Join(r.root, rel))
	if err != nil {
		return nil, err
	}
	base := filepath.Join(state, baseName)
	if resolvedBase != "" {
		base = resolvedBase
	}
	bm, err := dal.SyncTree(base)
	if err != nil {
		return nil, err
	}
	if bm.Digest != baseDigest {
		return nil, fmt.Errorf("corrupt_baseline")
	}
	// Community trees should be forked before editing. Import normalization is
	// retained as a local difference so invalid raw metadata stays reproducible.
	work, err := os.MkdirTemp("", "skm-update-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	for _, item := range []struct{ src, name string }{{base, "base"}, {e.Path, "local"}, {upstream, "upstream"}} {
		if err = dal.CopySyncTree(item.src, filepath.Join(work, item.name)); err != nil {
			return nil, err
		}
	}
	lm, err := dal.SyncTree(filepath.Join(work, "local"))
	if err != nil {
		return nil, err
	}
	if rec.Active == "upstream" {
		normalized := filepath.Join(work, "normalized-base")
		if err = dal.CopySyncTree(filepath.Join(work, "base"), normalized); err != nil {
			return nil, err
		}
		if err = normalizeStagedSkill(normalized, rec.Name); err != nil {
			return nil, err
		}
		expected, err := dal.SyncTree(normalized)
		if err != nil {
			return nil, err
		}
		if expected.Digest != lm.Digest {
			return nil, fmt.Errorf("customization_requires_fork: community directory was edited; run skm fork %s before update", e.Name)
		}
	}
	um, err := dal.SyncTree(filepath.Join(work, "upstream"))
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = "sha256:" + um.Digest
	}
	pr, err := ApplyCustomization(ctx, filepath.Join(work, "base"), filepath.Join(work, "local"), filepath.Join(work, "upstream"), filepath.Join(work, "candidate"))
	if err != nil {
		return nil, err
	}
	patch, err := GenerateCustomizationPatch(ctx, filepath.Join(work, "base"), filepath.Join(work, "local"))
	if err != nil {
		return nil, err
	}
	if err = dal.WriteSyncFile(filepath.Join(work, "customization.patch"), patch, 0600); err != nil {
		return nil, err
	}

	if err = dal.WriteSyncJSON(filepath.Join(work, "local-manifest.json"), lm); err != nil {
		return nil, err
	}
	operationID, err := newOperationID()
	if err != nil {
		return nil, err
	}
	attempt := SyncAttempt{Schema: 2, OperationID: operationID, Entry: rel, Generation: rec.Generation, RecordDigest: recordHash, UpstreamDigest: upHash, LocalDigest: lm.Digest, Version: version, BaseDigest: bm.Digest, NewDigest: um.Digest, Failures: pr.Failures}
	if err = dal.WriteSyncJSON(filepath.Join(work, "attempt.json"), attempt); err != nil {
		return nil, err
	}
	if candidateErr := r.validateCandidate(filepath.Join(work, "candidate"), rec.Name); candidateErr != nil {
		pr.Failures = append(pr.Failures, SyncChange{Path: "SKILL.md", Kind: "invalid_candidate: " + candidateErr.Error()})
		attempt.Failures = pr.Failures
		if err = dal.WriteSyncJSON(filepath.Join(work, "attempt.json"), attempt); err != nil {
			return nil, err
		}
	}
	result := &UpdateResult{Before: lm.Digest, Outcome: "updated", BaselineVersion: rec.UpstreamVersion, UpstreamVersion: version, Failures: pr.Failures}
	if len(pr.Failures) > 0 {
		result.Outcome = "manual_required"
		if !dryRun {
			if err = dal.InitSyncStore(r.root); err != nil {
				return result, err
			}
			path := filepath.Join(r.root, ".skm", "work", "operations", attempt.OperationID)
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return result, err
			}
			temp, err := os.MkdirTemp(filepath.Dir(path), ".pending-*")
			if err != nil {
				return result, err
			}
			defer func() { _ = os.RemoveAll(temp) }()
			if err = dal.CopySyncTree(work, temp); err != nil {
				return result, err
			}
			if err = os.Rename(temp, path); err != nil {
				return result, err
			}
			result.AttemptPath = path
		}
		return result, fmt.Errorf("patch_failed: manual intervention required; see info %s", e.Name)
	}
	return r.acceptAttempt(e, work, attempt, result, dryRun)
}

func (r *Repository) validateCandidate(path, name string) error {
	actual, err := frontmatterName(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return err
	}
	if actual != name {
		return fmt.Errorf("invalid_candidate: expected %q, got %q", name, actual)
	}
	m, err := dal.SyncTree(path)
	if err != nil {
		return err
	}
	_ = m

	return nil
}
func (r *Repository) acceptAttempt(e *common.Entry, work string, a SyncAttempt, result *UpdateResult, dryRun bool) (*UpdateResult, error) {
	rec, rel, err := r.SyncRecord(e)
	if err != nil {
		return result, err
	}
	if a.Schema != 2 || a.Entry != rel || a.Generation == 0 {
		return result, fmt.Errorf("invalid attempt")
	}
	for _, item := range []struct{ path, digest string }{{"base", a.BaseDigest}, {"local", a.LocalDigest}, {"upstream", a.NewDigest}} {
		m, err := dal.SyncTree(filepath.Join(work, item.path))
		if err != nil {
			return result, err
		}
		if m.Digest != item.digest {
			return result, fmt.Errorf("corrupt attempt %s", item.path)
		}
	}
	state, oldState, err := r.recordStage(rel)
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(state) }()
	current, err := dal.TreeFingerprint(e.Path)
	if err != nil {
		return result, err
	}
	up, err := dal.TreeFingerprint(filepath.Join(r.root, rel))
	if err != nil {
		return result, err
	}
	if current != a.LocalDigest || oldState != a.RecordDigest || up != a.UpstreamDigest || rec.Generation != a.Generation {
		return result, fmt.Errorf("stale_input: official content or association changed")
	}
	candidate := filepath.Join(work, "candidate")
	for _, failure := range a.Failures {
		if failure.MarkerLabel == "" {
			continue
		}
		file, err := dal.SyncPath(candidate, filepath.FromSlash(failure.Path))
		if err != nil {
			return result, err
		}
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if strings.Contains(string(data), failure.MarkerLabel) {
			return result, fmt.Errorf("invalid_candidate: unresolved patch markers in %s", failure.Path)
		}
	}

	if err = r.validateCandidate(candidate, rec.Name); err != nil {
		return result, err
	}
	cm, err := dal.SyncTree(candidate)
	if err != nil {
		return result, err
	}
	upstream := filepath.Join(work, "upstream")
	if err = replaceTree(upstream, filepath.Join(state, "upstream-baseline")); err != nil {
		return result, err
	}
	rec.UpstreamDigest = a.NewDigest
	rec.UpstreamVersion = a.Version
	rec.Origin.Revision = a.Version
	rec.UpstreamBase = baselineRef(rec.Origin, a.NewDigest)
	rec.Generation++
	community := candidate
	var replacements []dal.SyncReplacement
	if rec.Active == "fork" {
		community = filepath.Join(work, "community")
		if err = dal.CopySyncTree(upstream, community); err != nil {
			return result, err
		}
		if err = normalizeStagedSkill(community, rec.Name); err != nil {
			return result, err
		}
		if err = r.validateCandidate(community, rec.Name); err != nil {
			return result, err
		}
		if err = replaceTree(upstream, filepath.Join(state, "fork-baseline")); err != nil {
			return result, err
		}
		rec.ForkDigest = a.NewDigest
		rec.ForkVersion = a.Version
		forkBase := baselineRef(rec.Origin, a.NewDigest)
		rec.ForkBase = &forkBase
		replacements = append(replacements, dal.SyncReplacement{Path: rec.ForkPath, Source: candidate, Expected: a.LocalDigest})
	}
	if err = dal.WriteSyncJSON(filepath.Join(state, "record.json"), rec); err != nil {
		return result, err
	}
	replacements = append(replacements, dal.SyncReplacement{Path: rel, Source: community, Expected: a.UpstreamDigest}, dal.SyncReplacement{Path: dal.SyncStateRel(rel), Source: state, Expected: oldState})
	result.Before = a.LocalDigest
	result.After = cm.Digest
	result.Changed = result.Before != result.After
	result.UpstreamVersion = a.Version
	result.ActivePath = e.Path
	if !result.Changed && a.Version == result.BaselineVersion {
		result.Outcome = "current"
	} else {
		result.Outcome = "updated"
	}
	if dryRun {
		return result, nil
	}
	if result.Outcome == "current" {
		return result, nil
	}
	if err = dal.CommitSync(r.root, rel, replacements); err != nil {
		result.Accepted = dal.IsSyncAccepted(err)
		return result, err
	}
	result.Accepted = true
	return result, nil
}

func (r *Repository) FinishSyncUpdate(e *common.Entry, opts SyncOptions) (*UpdateResult, error) {
	_, rel, err := r.SyncRecord(e)
	if err != nil {
		return nil, err
	}
	result := &UpdateResult{Outcome: "cancelled"}
	path, _, err := r.operationAttempt(rel)
	if err != nil {
		return result, err
	}
	if path == "" {
		return result, fmt.Errorf("rollback_unavailable: no unfinished update attempt")
	}
	if opts.Abort || opts.Rollback {
		var attempt SyncAttempt
		if err = dal.ReadSyncJSON(filepath.Join(path, "attempt.json"), &attempt); err != nil {
			if opts.Rollback {
				if err = dal.RollbackSync(r.root, rel, opts.DryRun); err == nil {
					result.Outcome = "rolled_back"
					return result, nil
				}
			}
			return result, fmt.Errorf("rollback_unavailable: %w", err)
		}
		current, hashErr := dal.TreeFingerprint(e.Path)
		if hashErr != nil {
			return result, hashErr
		}
		if current != attempt.LocalDigest {
			return result, fmt.Errorf("stale_input: official content changed after update attempt")
		}
		result.Outcome = "rolled_back"
		if opts.DryRun {
			return result, nil
		}
		return result, os.RemoveAll(path)
	}
	var a SyncAttempt
	if err = dal.ReadSyncJSON(filepath.Join(path, "attempt.json"), &a); err != nil {
		return result, err
	}
	work, err := os.MkdirTemp("", "skm-accept-*")
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err = dal.CopySyncTree(path, work); err != nil {
		return result, err
	}
	result.Outcome = "updated"
	result, err = r.acceptAttempt(e, work, a, result, opts.DryRun)
	if err == nil && !opts.DryRun {
		err = os.RemoveAll(path)
	}
	return result, err
}
