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

// Capability describes what this provider handles. For skills.sh, the
// advertised schemes are the two copy-paste forms its own site actually
// shows (an npx install command, a skill's page URL) rather than this
// provider's own "skills.sh://" scheme — that scheme is still accepted by
// CanHandle/Fetch (parseSkillsShShortcut's error message points to it as an
// exact-path fallback), it's just not something anyone is ever handed to
// paste, so it isn't advertised.
func (g gitBackedProvider) Capability() Capability {
	schemes := []string{g.scheme}
	description := fmt.Sprintf("Fetches git-backed assets from %s<path> addresses", g.scheme)
	if g.id == "skills-sh" {
		schemes = []string{
			"npx skills add <repo-url> --skill <name>",
			"https://skills.sh/<owner>/<repo>/<name>",
		}
		description = "Fetches a skill named on a skills.sh page — paste either its npx " +
			"install command or the page URL itself — by searching the repo for a " +
			"matching directory"
	}
	return Capability{
		ID: g.id, Label: g.label,
		Description: description,
		Schemes:     schemes,
		Icon:        g.icon,
	}
}

// CanHandle reports whether address uses this provider's scheme, or (for
// skills.sh) one of parseSkillsShShortcut's copy-paste forms.
func (g gitBackedProvider) CanHandle(address string) bool {
	if strings.HasPrefix(address, g.scheme) {
		return true
	}
	return g.id == "skills-sh" && parseSkillsShShortcut(address) != nil
}

// Normalize returns the address unchanged. Rewriting it to a plain clone URL
// here would strip both the scheme (which CanHandle and Fetch require to
// claim and parse the address) and any subdirectory (which a clone URL has
// no way to carry) before Fetch — the only place with the machinery to
// resolve either — ever sees them; import_service.fetchProvider calls Fetch
// with Normalize's result, not the original address. Fetch resolves the real
// clone URL itself, exactly like GitHub's owner/repo/subdir shorthand leaves
// Normalize a no-op for the same reason.
func (g gitBackedProvider) Normalize(address string) (string, error) {
	return address, nil
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
// browse-URL/subpath-shorthand support uses (provider_git_weburl.go). A
// skills.sh shortcut (parseSkillsShShortcut) instead searches the clone by
// name, since it carries no path.
func (g gitBackedProvider) Fetch(ctx context.Context, address string) (string, error) {
	if g.id == "skills-sh" {
		if sc := parseSkillsShShortcut(address); sc != nil {
			return g.cloneAndStage(ctx, sc.repoURL, func(work string) (string, error) {
				return findSkillDirectory(work, sc.name)
			})
		}
	}
	if !strings.HasPrefix(address, g.scheme) {
		return "", &ProviderError{Code: CodeUnsupportedAddress,
			Message: fmt.Sprintf("%s: address %q does not use the %s scheme", g.id, address, g.scheme)}
	}
	repoPath, subdir := splitRepoSubpath(strings.TrimPrefix(address, g.scheme))
	repoURL := fmt.Sprintf("https://%s/%s.git", g.resolveHost(), repoPath)

	if subdir == "" {
		tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
		if err != nil {
			return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: create temp dir: %s", g.id, err)}
		}
		if err := g.clone(ctx, repoURL, tmp); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
		return tmp, nil
	}

	return g.cloneAndStage(ctx, repoURL, func(work string) (string, error) {
		src, err := containedPath(work, subdir)
		if err != nil {
			return "", err
		}
		if fi, serr := os.Stat(src); serr != nil || !fi.IsDir() {
			return "", fmt.Errorf("%s has no directory %q", repoPath, subdir)
		}
		return src, nil
	})
}

// cloneAndStage clones repoURL to a side directory, resolves the directory to
// keep via locate, and moves just that into a fresh temp dir — the caller
// frees exactly the path returned here, so the clone cannot stay wrapped
// around it (mirrors gitHostProvider.Fetch).
func (g gitBackedProvider) cloneAndStage(ctx context.Context, repoURL string, locate func(work string) (string, error)) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
	if err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: create temp dir: %s", g.id, err)}
	}
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
	src, err := locate(work)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: %s", g.id, err)}
	}
	if err := os.RemoveAll(tmp); err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: stage: %s", g.id, err)}
	}
	if err := os.Rename(src, tmp); err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: stage: %s", g.id, err)}
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
