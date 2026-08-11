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
	staged, providerID, group, origin, cleanup, err := s.resolveSource(ctx, source, opts.Provider)
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
		dest := filepath.Join(s.Cfg.Root, kind.TopDir(), providerID, group, name)
		return &ImportResult{Name: name, Type: kind, Provider: providerID, Path: dest, Origin: origin}, nil
	}

	res, err := s.Repo.ImportStaged(ctx, staged, providerID, group, opts.Force, origin)
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

// ClaimSkill adopts a recoverable skill directory already inside this
// repository into the self-build provider bucket. Unlike Import, this does
// not acquire content from a Provider: self-build entries are local work the
// user owns, and SelfBuild intentionally has no Fetch implementation.
func (s *Services) ClaimSkill(ctx context.Context, source string) (*ImportResult, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("claim: resolve source: %w", err), common.ExitError)
	}
	if _, inside := repoRelative(abs, s.Cfg.Root); !inside {
		return nil, common.WithExitCode(fmt.Errorf("claim: %q is outside repository %q", source, s.Cfg.Root), common.ExitError)
	}
	kind, _, err := s.Repo.ProbeStaged(abs)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	if kind != common.KindSkill {
		return nil, common.WithExitCode(fmt.Errorf("claim: %q is a %s, not a skill", source, kind), common.ExitError)
	}
	res, err := s.Repo.ImportStaged(ctx, abs, "self-build", "", false, nil)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Name: res.Name, Type: res.Kind, Provider: res.Provider, Path: res.Path, Origin: res.Origin}, nil
}

// resolveSource picks the provider and staged content: explicit --provider,
// else a real local source as a local import, else the first matching
// provider (FR-020).
func (s *Services) resolveSource(ctx context.Context, source, providerID string) (staged, id, group string, origin *common.Origin, cleanup func(), err error) {
	cleanup = func() {}
	source = normalizeImportSource(source)
	source = skillDirectorySource(source)
	localSource := isLocalSource(source)
	if localSource && isManagedRepositoryEntry(source, s.Cfg.Root) {
		return "", "", "", nil, cleanup, common.WithExitCode(
			fmt.Errorf("import: local source %q is inside target repository %q; use install, move, or claim instead", source, s.Cfg.Root),
			common.ExitObject)
	}
	// A local source is borrowed from the caller, never acquired into a temp
	// directory. Treat both auto and an explicit local selection identically so
	// Import's deferred cleanup can never remove the caller's files.
	if localSource && (providerID == "" || providerID == "local") {
		modeID := "local"
		return source, modeID, "", &common.Origin{Address: source, ProviderID: &modeID}, cleanup, nil
	}
	if providerID != "" {
		p := s.Registry.Get(providerID)
		if p == nil {
			return "", "", "", nil, cleanup, common.WithExitCode(fmt.Errorf("import: unknown provider %q", providerID), common.ExitError)
		}
		return s.fetchProvider(ctx, p, source)
	}
	p := s.Registry.Match(source)
	if p == nil {
		return "", "", "", nil, cleanup, common.WithExitCode(
			fmt.Errorf("import: no provider can handle %q", source), common.ExitError)
	}
	return s.fetchProvider(ctx, p, source)
}

// skillDirectorySource treats a local SKILL.md address as its enclosing skill
// directory. A skill is a directory asset, and importing its marker file as a
// standalone Markdown file would otherwise incorrectly create a command.
// Remote URLs are left alone because they do not exist on the local filesystem.
func skillDirectorySource(source string) string {
	if filepath.Base(source) != "SKILL.md" {
		return source
	}
	fi, err := os.Stat(source)
	if err != nil || fi.IsDir() {
		return source
	}
	return filepath.Dir(source)
}

// repoRelative returns path relative to root, and whether it is inside root at
// all. It is the single containment check shared by claim (which requires
// inside) and import (which rejects managed locations).
func repoRelative(path, root string) (rel string, inside bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel, err = filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// isManagedRepositoryEntry reports whether path is already in a standard
// managed entry location. Those entries must not be re-imported. Non-standard
// paths inside the repository remain eligible for the existing claim/repair
// workflow.
func isManagedRepositoryEntry(path, root string) bool {
	rel, inside := repoRelative(path, root)
	if !inside {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) >= 3 && (parts[0] == "skills" || parts[0] == "commands")
}

// normalizeImportSource makes paths entered outside a shell behave like paths
// entered at a shell prompt. TUI input does not expand ~/..., and pasted paths
// can contain surrounding whitespace. Remote addresses are only trimmed.
func normalizeImportSource(source string) string {
	source = strings.TrimSpace(source)
	if source != "~" && !strings.HasPrefix(source, "~/") {
		return source
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return source
	}
	if source == "~" {
		return home
	}
	return filepath.Join(home, source[2:])
}

// grouper is implemented by providers that derive a natural sub-directory
// grouping from the source address — currently gitHostProvider's owner/repo
// (provider_github.go) — so an import lands under <provider>/<group>/<name>
// instead of flat <provider>/<name>. It's an optional capability (type
// assertion, not part of the Provider interface) so plugin providers and
// every other built-in are unaffected.
type grouper interface {
	Group(address string) string
}

func (s *Services) fetchProvider(ctx context.Context, p Provider, source string) (staged, id, group string, origin *common.Origin, cleanup func(), err error) {
	normalized, nerr := p.Normalize(source)
	if nerr != nil {
		return "", "", "", nil, func() {}, common.WithExitCode(nerr, common.ExitError)
	}
	tmp, ferr := p.Fetch(ctx, normalized)
	if ferr != nil {
		return "", "", "", nil, func() {}, common.WithExitCode(ferr, common.ExitError)
	}
	modeID := p.ID()
	if g, ok := p.(grouper); ok {
		group = g.Group(normalized)
	}
	return tmp, p.ID(), group, &common.Origin{Address: normalized, ProviderID: &modeID}, fetchCleanup(p, tmp), nil
}

// fetchCleanup frees what Fetch staged. A borrowed source (borrowsSource) is
// the caller's own path, so there is nothing to free — removing it would
// delete the user's files, including on the error paths that run cleanup after
// a failed probe.
func fetchCleanup(p Provider, staged string) func() {
	if borrowsSource(p) {
		return func() {}
	}
	return func() { _ = os.RemoveAll(staged) }
}

// isLocalSource reports whether source is a real local path the import should
// treat as a local import: an existing directory, or a .md file carrying
// frontmatter.
func isLocalSource(source string) bool {
	source = normalizeImportSource(source)
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
