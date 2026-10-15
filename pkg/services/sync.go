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
	result, err := s.Repo.MigrateLegacy(accept, expected, dryRun)
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

func (s *Services) SyncBaseline(ctx context.Context, name string, opts BaselineOptions) (*BaselineResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if err = s.Repo.SyncReadCheck(); err != nil {
		return nil, err
	}
	e, err := s.syncEntry(name)
	if err != nil {
		return nil, err
	}
	if opts.Source == "" || opts.Revision == "" {
		return nil, fmt.Errorf("baseline requires --from and immutable --revision")
	}
	if e.RepositoryPath != "" && !opts.NewStart {
		st, _ := s.Repo.SyncStatus(e)
		if st == nil || st.BaselineState == "valid" {
			return nil, fmt.Errorf("valid association exists; use --new-start explicitly")
		}
	}
	source := opts.Source
	id := opts.Provider
	if id == "" && e.Origin != nil {
		if e.Origin.ProviderID != nil {
			id = *e.Origin.ProviderID
		}
	}
	var staged string
	var cleanup func()
	var origin *common.Origin
	var cacheRepository, cacheRevision, cacheSubpath, cacheSource string
	p := s.Registry.Get(id)
	if p == nil {
		p = s.Registry.Match(source)
	}
	if vp, ok := p.(providers.VersionedFetcher); ok {
		got, err := vp.FetchVersioned(ctx, providers.FetchRequest{Address: source, Revision: opts.Revision, Subpath: opts.Subpath})
		if err != nil {
			return nil, err
		}
		cleanup = fetchCleanup(p, got.Path)
		defer cleanup()
		staged = filepath.Join(got.Path, got.ResolvedSubpath)
		cacheRepository, cacheRevision, cacheSubpath, cacheSource = got.RepositoryAddress, got.Revision, got.ResolvedSubpath, got.Path
		pid := p.ID()
		origin = &common.Origin{Address: source, ProviderID: &pid, Subpath: got.ResolvedSubpath, RepositoryAddress: got.RepositoryAddress, RequestedRef: got.RequestedRef, Revision: got.Revision}
	} else {
		var group string
		staged, _, group, origin, cleanup, err = s.resolveSource(ctx, source, id)
		_ = group
		if err != nil {
			return nil, err
		}
		defer cleanup()
		if opts.Subpath != "" {
			staged, err = dal.SyncPath(staged, opts.Subpath)
			if err != nil {
				return nil, err
			}
			origin.Subpath = opts.Subpath
		}
	}
	m, err := dal.SyncTree(staged)
	if err != nil {
		return nil, err
	}
	actual := origin.Revision
	if actual == "" {
		actual = "sha256:" + m.Digest
	}
	if opts.Revision != actual {
		return nil, fmt.Errorf("requested revision does not match fetched content: %s", actual)
	}
	local, err := dal.SyncTree(e.Path)
	if err != nil {
		return nil, err
	}
	res := &BaselineResult{Tree: m.Digest, Local: local.Digest, Revision: actual, Mode: "historical"}
	if opts.NewStart {
		res.Mode = "new-start"
	}
	if !opts.Accept || opts.DryRun {
		return res, nil
	}
	if opts.ExpectedTree != res.Tree || opts.ExpectedLocal != res.Local {
		return res, fmt.Errorf("stale_input: --accept requires preview --expected-tree and --expected-local")
	}
	if cacheRevision != "" {
		cached, cacheErr := dal.StoreGitCache(s.Cfg.Root, cacheRepository, cacheRevision, cacheSource)
		if cacheErr != nil {
			return res, cacheErr
		}
		staged = filepath.Join(cached, cacheSubpath)
	}
	origin.Revision = actual
	if err = s.Repo.AssociateBaseline(e, staged, origin, opts.NewStart); err != nil {
		return res, err
	}
	res.Accepted = true
	next, err := s.syncEntry(name)
	if err != nil {
		return res, err
	}
	report := &engines.UpdateResult{Accepted: true}
	return res, s.refreshSyncInstalls(next, report)
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
	path, err := filepath.EvalSymlinks(filepath.Join(root, subpath))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("historical_source_unavailable: source subpath escapes cache")
	}
	return path, nil
}
