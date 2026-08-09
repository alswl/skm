package services

import (
	"net/url"
	"path"
	"strings"
)

// gitWebLocation is what a GitHub/GitLab *web* URL points at: the repository
// behind it, the ref it was browsed at, and the directory inside it.
//
// These URLs are what the browser address bar gives you, so they are the
// natural way to refer to a skill that lives in a subdirectory of a larger
// repository — but they are not clonable. Handing one to git fails outright
// ("alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md is not a valid
// repository name"), so the provider takes it apart instead.
type gitWebLocation struct {
	repoPath string // "owner/repo"
	ref      string // branch or tag the URL was browsed at
	subdir   string // directory to stage; "" when the URL addresses the repository root
}

// webURLKinds are the path segments GitHub and GitLab use for browsing
// content. "blob" and "raw" address a file, so the directory holding it is
// what gets staged; "tree" already addresses a directory.
var webURLKinds = map[string]bool{"blob": true, "tree": true, "raw": true}

// parseGitWebURL reports whether address is a content-browsing URL on host and
// if so takes it apart. Anything else — a clone URL, an SCP-style address, an
// owner/repo shorthand, or some other page on the same host (a release, an
// issue) — is left for the plain clone path.
//
// A ref containing slashes ("release/1.x") is indistinguishable from the start
// of the path in these URLs; like every other tool that reads them, this takes
// the single segment after blob/tree/raw as the ref.
func parseGitWebURL(address, host string) (gitWebLocation, bool) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() != host {
		return gitWebLocation{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return gitWebLocation{}, false // owner/repo plus at least a kind and a ref
	}
	owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
	rest := parts[2:]
	if rest[0] == "-" {
		rest = rest[1:] // GitLab browses under /-/blob/… and /-/tree/…
	}
	if len(rest) < 2 || owner == "" || repo == "" {
		return gitWebLocation{}, false
	}
	kind, ref := rest[0], rest[1]
	if !webURLKinds[kind] || ref == "" {
		return gitWebLocation{}, false
	}
	sub := path.Join(rest[2:]...)
	if kind != "tree" {
		sub = path.Dir(sub) // blob/raw name a file; stage the directory holding it
	}
	if sub == "." || sub == "/" {
		sub = ""
	}
	return gitWebLocation{repoPath: owner + "/" + repo, ref: ref, subdir: sub}, true
}

// parseOwnerRepoSubpath detects "owner/repo/sub/dir…" — three or more
// "/"-separated segments, none empty, no whitespace, and not a URL (which
// contains "://" or "git@host:" and so is left to parseGitWebURL/gitURLHost
// instead). It extends the exact two-segment "owner/repo" shorthand
// (isOwnerRepoShorthand) with a path inside the repository, the same
// convenience a browse URL gives you without having to paste a full URL.
//
// Callers gate this to GitHub only: a GitHub repository path is always
// exactly two segments, so segment three onward can never be anything but a
// subdirectory. GitLab nests groups ("group/subgroup/project" is itself a
// valid repository path), so the same rule there would misparse a real
// repository as owner+subdir — a GitLab user who wants a subdirectory already
// has the browse-URL form (parseGitWebURL).
func parseOwnerRepoSubpath(s string) (repoPath, subdir string, ok bool) {
	if strings.ContainsAny(s, " \t") || strings.Contains(s, "://") || strings.HasPrefix(s, "git@") {
		return "", "", false
	}
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return "", "", false
	}
	for _, p := range parts {
		if p == "" {
			return "", "", false
		}
	}
	return parts[0] + "/" + parts[1], strings.Join(parts[2:], "/"), true
}
