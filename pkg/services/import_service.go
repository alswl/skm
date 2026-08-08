package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// ImportOptions controls an import action.
type ImportOptions struct {
	// Provider selects a provider by id explicitly.
	Provider string
	// Kind is an optional explicit kind hint ("auto" | "skill" | "command").
	Kind   string
	Force  bool
	DryRun bool
}

// ImportResult is the CLI JSON report for import (contract/cli-json.md).
type ImportResult struct {
	Name     string           `json:"name"`
	Type     common.EntryKind `json:"type"`
	Provider string           `json:"provider"`
	Path     string           `json:"path"`
	Origin   *common.Origin   `json:"origin"`
}

// Import acquires an asset from a local source or a provider address (FR-020,
// FR-021, FR-022). Auto mode prefers a real local source; otherwise providers
// match in registration order.
func (s *Services) Import(ctx context.Context, source string, opts ImportOptions) (*ImportResult, error) {
	staged, providerID, origin, cleanup, err := s.resolveSource(ctx, source, opts.Provider)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Probe once: validates the asset and enforces the --kind hint.
	kind, name, err := s.Repo.ProbeStaged(staged)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if opts.Kind == "skill" && kind != common.KindSkill {
		return nil, common.WithExitCode(fmt.Errorf("import: --kind skill but source is a %s", kind), common.ExitError)
	}
	if opts.Kind == "command" && kind != common.KindCommand {
		return nil, common.WithExitCode(fmt.Errorf("import: --kind command but source is a %s", kind), common.ExitError)
	}

	if opts.DryRun {
		dest := filepath.Join(s.Cfg.Root, kind.TopDir(), providerID, name)
		return &ImportResult{Name: name, Type: kind, Provider: providerID, Path: dest, Origin: origin}, nil
	}

	res, err := s.Repo.ImportStaged(ctx, staged, providerID, opts.Force, origin)
	if err != nil {
		return nil, err
	}
	return &ImportResult{
		Name:     res.Name,
		Type:     res.Kind,
		Provider: res.Provider,
		Path:     res.Path,
		Origin:   res.Origin,
	}, nil
}

// resolveSource picks the provider and staged content: explicit --provider,
// else a real local source as a local import, else the first matching
// provider (FR-020).
func (s *Services) resolveSource(ctx context.Context, source, providerID string) (staged string, id string, origin *common.Origin, cleanup func(), err error) {
	cleanup = func() {}
	if providerID != "" {
		p := s.Registry.Get(providerID)
		if p == nil {
			return "", "", nil, cleanup, common.WithExitCode(fmt.Errorf("import: unknown provider %q", providerID), common.ExitError)
		}
		return s.fetchProvider(ctx, p, source)
	}
	if isLocalSource(source) {
		return source, "local", nil, cleanup, nil
	}
	p := s.Registry.Match(source)
	if p == nil {
		return "", "", nil, cleanup, common.WithExitCode(
			fmt.Errorf("import: no provider can handle %q", source), common.ExitError)
	}
	return s.fetchProvider(ctx, p, source)
}

func (s *Services) fetchProvider(ctx context.Context, p Provider, source string) (staged, id string, origin *common.Origin, cleanup func(), err error) {
	tmp, ferr := p.Fetch(ctx, source)
	if ferr != nil {
		return "", "", nil, func() {}, common.WithExitCode(ferr, common.ExitError)
	}
	modeID := p.ID()
	return tmp, p.ID(), &common.Origin{Address: source, ProviderID: &modeID}, func() { _ = os.RemoveAll(tmp) }, nil
}

// isLocalSource reports whether source is a real local path the import should
// treat as a local import: an existing directory, or a .md file carrying
// frontmatter.
func isLocalSource(source string) bool {
	if !dal.PathExists(source) {
		return false
	}
	fi, err := os.Stat(source)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return true
	}
	if strings.HasSuffix(source, ".md") {
		data, err := os.ReadFile(source)
		if err != nil {
			return false
		}
		return dal.HasFrontmatter(data)
	}
	return false
}
