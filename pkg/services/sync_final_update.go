package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
)

func (s *Services) updateFinalSkill(ctx context.Context, e *common.Entry, meta *common.SkillMeta, opts UpdateOptions) (*engines.UpdateResult, error) {
	if meta.RevisionKind != "git" {
		return nil, fmt.Errorf("not_applicable: non-Git Skill is a fixed snapshot; import changed source as a new Skill")
	}
	if meta.Revision == "" {
		return nil, fmt.Errorf("migration_required: Git Skill has no exact recorded revision")
	}
	provider := s.Registry.Get(e.ProviderIDValue())
	versioned, ok := provider.(providers.VersionedFetcher)
	if !ok {
		return nil, fmt.Errorf("migration_required: origin provider cannot resolve exact Git history")
	}
	baseRevision := meta.Revision
	if meta.Active == "fork" {
		forkMeta, err := s.Repo.FinalForkMeta(e)
		if err != nil {
			return nil, err
		}
		baseRevision = forkMeta.BaseRevision
	}
	base, err := versioned.FetchVersioned(ctx, providers.FetchRequest{
		Address: meta.Address, RepositoryAddress: meta.RepositoryAddress,
		RequestedRef: meta.RequestedRef, Revision: baseRevision, Subpath: meta.Subpath,
	})
	if err != nil {
		return nil, fmt.Errorf("exact Git base %s unavailable: %w", baseRevision, err)
	}
	defer fetchCleanup(provider, base.Path)()
	fetched, err := s.fetchFromOrigin(ctx, e, false)
	if err != nil {
		return nil, err
	}
	defer fetched.Cleanup()
	if fetched.Version == "" {
		return nil, fmt.Errorf("git provider returned no immutable revision")
	}
	return s.Repo.PrepareFinalUpdate(ctx, e, meta, filepath.Join(base.Path, base.ResolvedSubpath), fetched.Path, fetched.Version, opts.DryRun)
}
