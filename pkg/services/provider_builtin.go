package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitBackedProvider is a built-in Provider for an internal source recognized
// by its own address scheme, resolved to a git URL and fetched via the
// system git binary — the same mechanism GitHub already uses.
//
// gitlab.go and skillssh.go are built on this shared helper: gitlab uses
// "gitlab://<org>/<repo>" against gitlab.com (SKM_GITLAB_HOST); skills-sh uses
// "skills.sh://<owner>/<repo>" against github.com (SKM_SKILLS_SH_HOST), since
// skills.sh indexes skills living in public GitHub repos. Each host is
// overridable per-provider via its EnvHostVar without any interface change.
type gitBackedProvider struct {
	id, label, scheme string
	envHostVar        string
	defaultHost       string
	icon              string
}

// ID returns the provider id.
func (g gitBackedProvider) ID() string { return g.id }

// Label returns the human label.
func (g gitBackedProvider) Label() string { return g.label }

// Capability describes what this provider handles.
func (g gitBackedProvider) Capability() Capability {
	return Capability{
		ID: g.id, Label: g.label,
		Description: fmt.Sprintf("Fetches git-backed assets from %s<path> addresses", g.scheme),
		Schemes:     []string{g.scheme},
		Icon:        g.icon,
	}
}

// CanHandle reports whether address uses this provider's scheme.
func (g gitBackedProvider) CanHandle(address string) bool {
	return strings.HasPrefix(address, g.scheme)
}

// Normalize resolves a scheme-prefixed address to its canonical git URL.
// An address that doesn't use this provider's scheme is returned unchanged
// (Local/GitHub convention: normalize is best-effort, not validation). A path
// naming a subdirectory (splitRepoSubpath) normalizes to its repository alone
// — a clone URL has no way to carry a subdirectory, that part only matters to
// Fetch.
func (g gitBackedProvider) Normalize(address string) (string, error) {
	if !g.CanHandle(address) {
		return address, nil
	}
	repoPath, _ := splitRepoSubpath(strings.TrimPrefix(address, g.scheme))
	return fmt.Sprintf("https://%s/%s.git", g.resolveHost(), repoPath), nil
}

// splitRepoSubpath splits a "owner/repo[/sub/dir]" path into the repository
// (its first two segments) and the directory inside it (everything after) —
// the same shape parseOwnerRepoSubpath detects for GitHub's shorthand.
// Callers here don't gate this the way GitHub does, because it doesn't need
// to: every gitBackedProvider address is an owner/repo path by construction
// (skills.sh indexes skills living in public GitHub repos — see NewSkillsSh),
// so there is no GitLab-style subgroup nesting to be ambiguous with. A path of
// fewer than two segments is returned as-is with no subdirectory; Fetch's
// clone then fails on it exactly as it already would have.
func splitRepoSubpath(path string) (repoPath, subdir string) {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 2 {
		// The trimmed form, not the input: a trailing slash reaching the clone
		// URL builds "https://host/owner/repo/.git", which git rejects.
		return trimmed, ""
	}
	return parts[0] + "/" + parts[1], strings.Join(parts[2:], "/")
}

// resolveHost returns the configured host override, or the reconstructed
// default (research R2).
func (g gitBackedProvider) resolveHost() string {
	if v := os.Getenv(g.envHostVar); v != "" {
		return v
	}
	return g.defaultHost
}

// Fetch clones the resolved git URL into a temp dir and returns its path. An
// address naming a subdirectory (splitRepoSubpath) clones the repository to
// the side and stages just that directory — the same technique GitHub's
// browse-URL/subpath-shorthand support uses (provider_git_weburl.go), for the
// same reason: a skill can live below the repository root.
func (g gitBackedProvider) Fetch(ctx context.Context, address string) (string, error) {
	if !g.CanHandle(address) {
		return "", &ProviderError{Code: CodeUnsupportedAddress,
			Message: fmt.Sprintf("%s: address %q does not use the %s scheme", g.id, address, g.scheme)}
	}
	repoPath, subdir := splitRepoSubpath(strings.TrimPrefix(address, g.scheme))
	repoURL := fmt.Sprintf("https://%s/%s.git", g.resolveHost(), repoPath)

	tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
	if err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: create temp dir: %s", g.id, err)}
	}
	if subdir == "" {
		if err := g.clone(ctx, repoURL, tmp); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
		return tmp, nil
	}

	// The caller frees exactly the path returned here, so the clone cannot
	// stay wrapped around it: clone to the side, then move the requested
	// directory into tmp and drop the rest (mirrors gitHostProvider.Fetch).
	work, err := os.MkdirTemp("", "skm-"+g.id+"-clone-*")
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: create temp dir: %s", g.id, err)}
	}
	defer func() { _ = os.RemoveAll(work) }()

	if err := g.clone(ctx, repoURL, work); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	src, err := containedPath(work, subdir)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: %s", g.id, err)}
	}
	if fi, serr := os.Stat(src); serr != nil || !fi.IsDir() {
		_ = os.RemoveAll(tmp)
		return "", &ProviderError{Code: CodeFetchFailed,
			Message: fmt.Sprintf("%s: %s has no directory %q", g.id, repoPath, subdir)}
	}
	if err := os.RemoveAll(tmp); err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: stage %q: %s", g.id, subdir, err)}
	}
	if err := os.Rename(src, tmp); err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: stage %q: %s", g.id, subdir, err)}
	}
	return tmp, nil
}

// clone shallow-clones url into dir.
func (g gitBackedProvider) clone(ctx context.Context, url, dir string) error {
	out, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, dir).CombinedOutput()
	if err != nil {
		return &ProviderError{Code: CodeFetchFailed,
			Message: fmt.Sprintf("%s: git clone %s: %s: %s", g.id, url, strings.TrimSpace(string(out)), err)}
	}
	return nil
}
