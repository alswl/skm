package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
)

type BaselineOptions struct {
	Source, Revision, Provider, Subpath, ExpectedTree, ExpectedLocal string
	NewStart, Accept, DryRun                                         bool
}
type BaselineResult struct {
	Tree, Local, Revision, Mode string
	Accepted                    bool
}

// MigrateSync is the only write path for the 011 storage layout. Preview is
// read-only; accepting binds to the digest returned by that preview.
func (s *Services) MigrateSync(ctx context.Context, accept bool, expected string, rollback bool, dryRun bool) (*engines.MigrationResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if rollback {
		result := &engines.MigrationResult{Outcome: "rolled_back", Preview: dryRun}
		if err := dal.RollbackSync(s.Cfg.Root, "migration", dryRun); err != nil {
			return result, err
		}
		result.Accepted = true
		return result, nil
	}
	if err = s.Repo.SyncReadCheck(); err != nil {
		return nil, err
	}
	var result *engines.MigrationResult
	if _, formatErr := os.Lstat(filepath.Join(s.Cfg.Root, ".skm", "format.json")); formatErr == nil {
		result, err = s.Repo.MigrateFinal(accept, expected, dryRun)
	} else if !os.IsNotExist(formatErr) {
		return nil, formatErr
	} else if dal.LegacySyncStore(s.Cfg.Root) {
		if preflightErr := s.Repo.PreflightFinalLegacy(); preflightErr != nil {
			return nil, preflightErr
		}
		result, err = s.Repo.MigrateLegacy(accept, expected, dryRun)
		if err == nil && result != nil && result.Accepted && !dryRun {
			finalPreview, previewErr := s.Repo.MigrateFinal(false, "", false)
			if previewErr != nil {
				return result, fmt.Errorf("legacy converted to readable v2; final migration preflight failed: %w", previewErr)
			}
			finalResult, finalErr := s.Repo.MigrateFinal(true, finalPreview.ExpectedState, false)
			if finalErr != nil {
				return result, fmt.Errorf("legacy converted to readable v2; final migration failed: %w", finalErr)
			}
			finalResult.ForkMoves = result.ForkMoves
			result = finalResult
		}
	} else {
		result, err = s.migrateMetaOnly(ctx, accept, expected, dryRun)
	}
	if err != nil || result == nil || !result.Accepted || dryRun {
		return result, err
	}
	for _, move := range result.ForkMoves {
		entry, resolveErr := s.ResolveEntry(move.Entry)
		if resolveErr != nil || entry == nil {
			result.Refresh = append(result.Refresh, engines.RefreshResult{Target: move.Entry, Success: false, Reason: "refresh_failed: migrated entry cannot be resolved"})
			continue
		}
		oldPath := filepath.Join(s.Cfg.Root, move.OldPath)
		for _, target := range s.Installer.LegacyForkTargets(entry, oldPath) {
			tx := &dal.FileTransaction{}
			_, installErr := s.Installer.Install(tx, entry, target, true)
			item := engines.RefreshResult{Target: target.Name, Success: installErr == nil}
			if installErr != nil {
				_ = tx.Rollback()
				item.Reason = installErr.Error()
			} else {
				tx.Commit()
			}
			result.Refresh = append(result.Refresh, item)
		}
	}
	return result, nil
}

func (s *Services) SyncBaseline(context.Context, string, BaselineOptions) (*BaselineResult, error) {
	return nil, common.WithExitCode(fmt.Errorf("sync baseline is retired; run skm sync migrate to convert legacy repositories, or import changed sources as a new Skill"), common.ExitObject)
}

func (s *Services) SyncDiff(ctx context.Context, name string) (string, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return "", err
	}
	defer lock.Release()
	if err = s.Repo.SyncReadCheck(); err != nil {
		return "", err
	}
	e, err := s.syncEntry(name)
	if err != nil {
		return "", err
	}
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	if meta, metaErr := dal.ReadSkillMeta(upstream); metaErr == nil && meta.SchemaVersion == 3 {
		return s.finalSyncDiff(ctx, e, meta)
	} else if metaErr != nil && !os.IsNotExist(metaErr) {
		return "", metaErr
	}
	rec, rel, err := s.Repo.SyncRecord(e)
	if err != nil {
		return "", fmt.Errorf("missing_baseline: %w", err)
	}
	kind := "upstream-baseline"
	ref := rec.UpstreamBase
	if rec.Active == "fork" {
		kind = "fork-baseline"
		if rec.ForkBase != nil {
			ref = *rec.ForkBase
		}
	}
	base, cleanup, err := s.resolveBaselineTree(ctx, rec, ref, filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), kind))
	if err != nil {
		return "", err
	}
	defer cleanup()
	st, err := s.Repo.SyncStatus(e)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, change := range st.Changes {
		fmt.Fprintf(&out, "%s %s\n", change.Kind, change.Path)
		before, _ := os.ReadFile(filepath.Join(base, change.Path))
		after, _ := os.ReadFile(filepath.Join(e.Path, change.Path))
		if !utf8.Valid(before) || !utf8.Valid(after) || strings.ContainsRune(string(before), 0) || strings.ContainsRune(string(after), 0) {
			out.WriteString("  binary content changed\n")
			continue
		}
		// Complete removed/added text is deliberately lossless, including deletions.
		if len(before) > 0 {
			fmt.Fprintf(&out, "--- baseline/%s\n-%s\n", change.Path, strings.ReplaceAll(string(before), "\n", "\n-"))
		}
		if len(after) > 0 {
			fmt.Fprintf(&out, "+++ local/%s\n+%s\n", change.Path, strings.ReplaceAll(string(after), "\n", "\n+"))
		}
	}
	return out.String(), nil
}

// resolveBaselineTree obtains a raw immutable baseline only for operations
// that need it. Catalog and install paths deliberately never call this.
func (s *Services) resolveBaselineTree(ctx context.Context, rec *common.SyncRecord, ref common.BaselineRef, snapshot string) (string, func(), error) {
	if ref.Kind != "git" {
		// New records keep non-reconstructible inputs in a content-addressed
		// snapshot. Keep the entry-sidecar fallback only for records created
		// before that layout was introduced.
		if ref.SnapshotDigest != "" {
			if rel, err := dal.SyncBaselineRel(ref.SnapshotDigest); err == nil {
				if canonical, pathErr := dal.SyncPath(s.Cfg.Root, rel); pathErr == nil {
					if m, treeErr := dal.SyncTree(canonical); treeErr == nil && m.Digest == ref.RawDigest {
						return canonical, func() {}, nil
					}
				}
			}
		}
		m, err := dal.SyncTree(snapshot)
		if err != nil {
			return "", func() {}, fmt.Errorf("missing_baseline: %w", err)
		}
		if ref.RawDigest != "" && m.Digest != ref.RawDigest {
			return "", func() {}, fmt.Errorf("missing_baseline: snapshot digest mismatch")
		}
		return snapshot, func() {}, nil
	}
	repository := rec.Origin.RepositoryAddress
	if repository == "" {
		repository = rec.Origin.Address
	}
	if cached, err := dal.ReadGitCache(s.Cfg.Root, repository, ref.Revision); err == nil {
		base, err := cacheSubpath(cached, rec.Origin.Subpath)
		if err != nil {
			return "", func() {}, err
		}
		m, err := dal.SyncTree(base)
		if err == nil && m.Digest == ref.RawDigest {
			return base, func() {}, nil
		}
	}
	providerID := ""
	if rec.Origin.ProviderID != nil {
		providerID = *rec.Origin.ProviderID
	}
	p := s.Registry.Get(providerID)
	if p == nil {
		return "", func() {}, fmt.Errorf("historical_source_unavailable: provider %q is unavailable", providerID)
	}
	vp, ok := p.(providers.VersionedFetcher)
	if !ok {
		return "", func() {}, fmt.Errorf("historical_source_unavailable: origin is not versioned")
	}
	got, err := vp.FetchVersioned(ctx, providers.FetchRequest{Address: rec.Origin.Address, RepositoryAddress: rec.Origin.RepositoryAddress, RequestedRef: rec.Origin.RequestedRef, Revision: ref.Revision, Subpath: rec.Origin.Subpath})
	if err != nil {
		return "", func() {}, fmt.Errorf("historical_source_unavailable: %w", err)
	}
	cleanup := fetchCleanup(p, got.Path)
	base := filepath.Join(got.Path, got.ResolvedSubpath)
	m, err := dal.SyncTree(base)
	if err != nil || m.Digest != ref.RawDigest {
		cleanup()
		if err != nil {
			return "", func() {}, err
		}
		return "", func() {}, fmt.Errorf("historical_source_unavailable: baseline digest mismatch")
	}
	cached, err := dal.StoreGitCache(s.Cfg.Root, got.RepositoryAddress, got.Revision, got.Path)
	cleanup()
	if err != nil {
		return "", func() {}, err
	}
	base, err = cacheSubpath(cached, got.ResolvedSubpath)
	if err != nil {
		return "", func() {}, err
	}
	return base, func() {}, nil
}

func cacheSubpath(root, subpath string) (string, error) {
	if subpath == "" || subpath == "." {
		return root, nil
	}
	if !filepath.IsLocal(subpath) {
		return "", fmt.Errorf("historical_source_unavailable: unsafe source subpath")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, subpath))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(canonicalRoot, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("historical_source_unavailable: source subpath escapes cache")
	}
	return path, nil
}
