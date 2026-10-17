package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/providers"
)

type legacyBaselineMatch struct {
	Path    string
	Origin  common.Origin
	Cleanup func()
}

// matchLegacyBaseline only accepts an exact content match. Old meta.json
// records did not save a revision or raw baseline, so a changed local tree
// cannot safely be classified as upstream drift or customization.
func (s *Services) matchLegacyBaseline(ctx context.Context, e *common.Entry) (*legacyBaselineMatch, error) {
	p := s.Registry.Get(e.ProviderIDValue())
	if p == nil {
		p = s.Registry.Match(e.Origin.Address)
	}
	fetched, err := s.fetchFromOrigin(ctx, e, false)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			fetched.Cleanup()
		}
	}()
	origin := *e.Origin
	if _, ok := p.(providers.VersionedFetcher); !ok {
		matched, err := s.Repo.LegacyContentMatches(e, fetched.Path)
		if err != nil || !matched {
			return nil, err
		}
		raw, err := dal.SyncTree(fetched.Path)
		if err != nil {
			return nil, err
		}
		origin.Revision = "sha256:" + raw.Digest
		keep = true
		return &legacyBaselineMatch{Path: fetched.Path, Origin: origin, Cleanup: fetched.Cleanup}, nil
	}

	clone, err := gitOutput(ctx, fetched.Path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	clone, err = filepath.EvalSymlinks(clone)
	if err != nil {
		return nil, err
	}
	sub, err := filepath.Rel(clone, fetched.Path)
	if err != nil || (!filepath.IsLocal(sub) && sub != ".") {
		return nil, fmt.Errorf("unsafe historical source path")
	}
	repository, err := gitOutput(ctx, clone, "config", "--get", "remote.origin.url")
	if err != nil {
		return nil, err
	}
	matched, err := s.Repo.LegacyContentMatches(e, fetched.Path)
	if err != nil {
		return nil, err
	}
	if matched {
		origin.RepositoryAddress = repository
		origin.Subpath = sub
		origin.Revision = fetched.Version
		keep = true
		return &legacyBaselineMatch{Path: fetched.Path, Origin: origin, Cleanup: fetched.Cleanup}, nil
	}
	shallow, err := gitOutput(ctx, clone, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if shallow == "true" {
		if _, err = gitOutput(ctx, clone, "fetch", "--unshallow", "origin"); err != nil {
			return nil, fmt.Errorf("historical_source_unavailable: %w", err)
		}
	}
	revisions, err := gitOutput(ctx, clone, "rev-list", "HEAD", "--", sub)
	if err != nil {
		return nil, err
	}
	for _, revision := range strings.Fields(revisions) {
		if _, err = gitOutput(ctx, clone, "checkout", "--detach", "--force", revision); err != nil {
			return nil, err
		}
		candidate := filepath.Join(clone, sub)
		if _, err = os.Lstat(candidate); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		matched, err := s.Repo.LegacyContentMatches(e, candidate)
		if err != nil {
			return nil, err
		}
		if matched {
			origin.RepositoryAddress = repository
			origin.Subpath = sub
			origin.Revision = revision
			keep = true
			return &legacyBaselineMatch{Path: candidate, Origin: origin, Cleanup: fetched.Cleanup}, nil
		}
	}
	return nil, nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git historical source: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}
