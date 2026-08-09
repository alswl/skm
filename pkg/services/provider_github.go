package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitHostProvider clones git repositories (URLs or owner/repo shorthand) into
// a temp staging directory using the system git binary. GitHub and GitLab
// share this exact access pattern — only the id/label/host differ — so both
// resolve owner/repo shorthand to their own https host and clone over https
// (plan.md constraints: no network except via git).
type gitHostProvider struct {
	id, label, host, description, icon string
	// allowSubpathShorthand permits "owner/repo/sub/dir" shorthand
	// (parseOwnerRepoSubpath) — true for GitHub only; see that function for
	// why GitLab's nested groups make the same rule ambiguous there.
	allowSubpathShorthand bool
}

// NewGitHub returns the built-in GitHub provider.
func NewGitHub() Provider {
	return gitHostProvider{
		id: "github", label: "GitHub / git repositories", host: "github.com",
		description:           "Clones git URLs (git@/https/ssh/.git) and owner/repo[/subdir] shorthand",
		icon:                  "🐙",
		allowSubpathShorthand: true,
	}
}

// NewGitLab returns the built-in GitLab provider — the same access pattern as
// GitHub, with gitlab.com as the shorthand host (http clone).
func NewGitLab() Provider {
	return gitHostProvider{
		id: "gitlab", label: "GitLab", host: "gitlab.com",
		description: "Clones git URLs (git@/https/ssh/.git) and owner/repo shorthand",
		icon:        "🦊",
	}
}

// ID returns the provider id.
func (g gitHostProvider) ID() string { return g.id }

// Label returns the human label.
func (g gitHostProvider) Label() string { return g.label }

// Capability describes what this provider handles.
func (g gitHostProvider) Capability() Capability {
	return Capability{
		ID: g.id, Label: g.label,
		Description: g.description,
		Schemes:     []string{"git@", "https://", "ssh://", "owner/repo"},
		Icon:        g.icon,
	}
}

// Normalize rewrites an owner/repo shorthand to the provider's https URL; real
// URLs are returned unchanged.
func (g gitHostProvider) Normalize(address string) (string, error) {
	return normalizeGitURL(address, g.host), nil
}

// CanHandle reports whether address is an owner/repo shorthand (host-agnostic
// by design; --provider or registration order picks the target host) or a git
// URL whose host is this provider's own (g.host) — a git URL for another host
// (e.g. an internal git server) must not be claimed here just because it looks
// git-shaped, or it never reaches a plugin provider registered for that host
// and its host-specific auth/fetch behavior is silently skipped.
func (g gitHostProvider) CanHandle(address string) bool {
	if isOwnerRepoShorthand(address) {
		return true
	}
	if g.allowSubpathShorthand {
		if _, _, ok := parseOwnerRepoSubpath(address); ok {
			return true
		}
	}
	return isGitURL(address) && gitURLHost(address) == g.host
}

// webOrSubpathLocation resolves address to a repository plus a directory
// inside it to stage, when address names one at all: a browse URL
// (parseGitWebURL), or — GitHub only — owner/repo/sub/dir shorthand
// (parseOwnerRepoSubpath). Shared by Group and Fetch so both take the
// address apart exactly the same way. ok is false for anything else — a plain
// clone URL or an exact owner/repo shorthand — which have no subdirectory to
// extract and are cloned whole.
func (g gitHostProvider) webOrSubpathLocation(address string) (loc gitWebLocation, ok bool) {
	if web, isWeb := parseGitWebURL(address, g.host); isWeb {
		return web, true
	}
	if g.allowSubpathShorthand {
		if repoPath, subdir, isSubpath := parseOwnerRepoSubpath(address); isSubpath {
			return gitWebLocation{repoPath: repoPath, subdir: subdir}, true
		}
	}
	return gitWebLocation{}, false
}

// Group derives the "owner/repo" sub-directory group from address (the same
// forms CanHandle accepts: owner/repo shorthand, or a git URL for this
// provider's own host), so an import lands under <provider>/<owner>/<repo>/
// <name> instead of a flat <provider>/<name> — multiple skills imported from
// different repos (or the same repo) are grouped and disambiguated in the
// list/detail views by more than name alone ("GitHub 要显示 group/repo/
// name"). Returns "" (no group; flat layout) when address doesn't resolve to
// exactly two path segments.
func (g gitHostProvider) Group(address string) string {
	// A browse URL or owner/repo/subdir shorthand carries a path on top of the
	// repository, so it never splits into exactly two segments on its own —
	// take it apart first, or the same skill would group differently
	// depending on which form of the address was pasted.
	if loc, ok := g.webOrSubpathLocation(address); ok {
		return loc.repoPath
	}
	// isGitURL is checked before the shorthand: a git@ SCP-style address like
	// "git@github.com:owner/repo.git" happens to split into exactly two
	// "/"-separated segments too (isOwnerRepoShorthand doesn't know about the
	// "git@host:" prefix), so checking shorthand first would misparse it.
	var path string
	switch {
	case isGitURL(address):
		if gitURLHost(address) != g.host {
			return ""
		}
		path = gitURLPath(address)
	case isOwnerRepoShorthand(address):
		path = address
	default:
		return ""
	}
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// gitURLPath extracts the path portion of a git URL, in either URL-scheme
// form (https://host/owner/repo, ssh://host/owner/repo, git://…) or SCP-style
// form (git@host:owner/repo.git). Returns "" if it can't be determined.
func gitURLPath(s string) string {
	if strings.HasPrefix(s, "git@") {
		rest := strings.TrimPrefix(s, "git@")
		if i := strings.IndexAny(rest, ":/"); i >= 0 {
			return rest[i+1:]
		}
		return ""
	}
	if u, err := url.Parse(s); err == nil {
		return strings.TrimPrefix(u.Path, "/")
	}
	return ""
}

// Fetch clones the address into a temp dir and returns its path. A browse URL
// or owner/repo/subdir shorthand (webOrSubpathLocation) is cloned at the ref
// it names (if any), and the directory it points inside becomes the staged
// result — that is how a skill living in a subdirectory of a larger
// repository gets imported.
func (g gitHostProvider) Fetch(ctx context.Context, address string) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
	if err != nil {
		return "", fmt.Errorf("%s: create temp dir: %w", g.id, err)
	}
	loc, hasLoc := g.webOrSubpathLocation(address)
	if !hasLoc || loc.subdir == "" {
		cloneURL := normalizeGitURL(address, g.host)
		var ref string
		if hasLoc {
			cloneURL, ref = "https://"+g.host+"/"+loc.repoPath, loc.ref
		}
		if err := g.clone(ctx, cloneURL, ref, tmp); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
		return tmp, nil
	}

	// The caller frees exactly the path returned here (fetchProvider's
	// cleanup), so the clone cannot stay wrapped around it: clone to the side,
	// then move the requested directory into tmp and drop the rest.
	work, err := os.MkdirTemp("", "skm-"+g.id+"-clone-*")
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("%s: create temp dir: %w", g.id, err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	if err := g.clone(ctx, "https://"+g.host+"/"+loc.repoPath, loc.ref, work); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	src, err := containedPath(work, loc.subdir)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("%s: %w", g.id, err)
	}
	if fi, serr := os.Stat(src); serr != nil || !fi.IsDir() {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("%s: %s has no directory %q at %s", g.id, loc.repoPath, loc.subdir, refOrDefault(loc.ref))
	}
	if err := os.RemoveAll(tmp); err != nil {
		return "", fmt.Errorf("%s: stage %q: %w", g.id, loc.subdir, err)
	}
	if err := os.Rename(src, tmp); err != nil {
		return "", fmt.Errorf("%s: stage %q: %w", g.id, loc.subdir, err)
	}
	return tmp, nil
}

// refOrDefault names the ref an error message should mention: the explicit
// ref a browse URL carried, or "the default branch" for owner/repo/subdir
// shorthand, which names none.
func refOrDefault(ref string) string {
	if ref == "" {
		return "the default branch"
	}
	return ref
}

// clone shallow-clones url into dir, at ref when one is given.
func (g gitHostProvider) clone(ctx context.Context, url, ref, dir string) error {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dir)
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: git clone %s: %s: %w", g.id, url, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// containedPath joins rel onto base, refusing anything that escapes base — the
// path comes from a URL, so ".." in it must not be able to reach outside the
// clone.
func containedPath(base, rel string) (string, error) {
	joined := filepath.Join(base, filepath.FromSlash(rel))
	if joined != base && !strings.HasPrefix(joined, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the repository", rel)
	}
	return joined, nil
}

// isGitURL detects common git URL forms.
func isGitURL(s string) bool {
	return strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "git://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasSuffix(s, ".git")
}

// gitURLHost extracts the host from a git URL, in either URL-scheme form
// (https://host/..., ssh://host/..., git://host/...) or SCP-style form
// (git@host:owner/repo.git). Returns "" if no host can be determined (a bare
// path ending in ".git", with no scheme or git@ prefix).
func gitURLHost(s string) string {
	if strings.HasPrefix(s, "git@") {
		rest := strings.TrimPrefix(s, "git@")
		if i := strings.IndexAny(rest, ":/"); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	if u, err := url.Parse(s); err == nil {
		return u.Hostname()
	}
	return ""
}

// isOwnerRepoShorthand detects "owner/repo".
func isOwnerRepoShorthand(s string) bool {
	parts := strings.Split(s, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(s, " \t")
}

// normalizeGitURL converts an owner/repo shorthand into the host's https URL
// and leaves real URLs untouched.
func normalizeGitURL(s, host string) string {
	if isOwnerRepoShorthand(s) {
		return "https://" + host + "/" + s
	}
	return s
}
