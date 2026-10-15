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
	if rec.Active == "fork" {
		kind = "fork-baseline"
	}
	base := filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), kind)
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
