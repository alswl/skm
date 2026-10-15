package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FetchRequest struct{ Address, RepositoryAddress, RequestedRef, Revision, Subpath string }
type FetchResult struct{ Path, RepositoryAddress, RequestedRef, ResolvedSubpath, Revision string }
type VersionedFetcher interface {
	FetchVersioned(context.Context, FetchRequest) (FetchResult, error)
}

func (g gitHostProvider) FetchVersioned(ctx context.Context, r FetchRequest) (FetchResult, error) {
	url, ref, sub := r.RepositoryAddress, r.RequestedRef, r.Subpath
	if url == "" {
		url = normalizeGitURL(r.Address, g.host)
		if loc, ok := g.webOrSubpathLocation(r.Address); ok {
			url = "https://" + g.host + "/" + loc.repoPath
			ref = loc.ref
			if r.Subpath == "" {
				sub = loc.subdir
			}
		}
	}
	return fetchGitVersion(ctx, url, ref, r.Revision, sub, nil)
}
func (g gitBackedProvider) FetchVersioned(ctx context.Context, r FetchRequest) (FetchResult, error) {
	if r.RepositoryAddress != "" {
		return fetchGitVersion(ctx, r.RepositoryAddress, r.RequestedRef, r.Revision, r.Subpath, nil)
	}
	if g.id == "skills-sh" {
		sc, err := parseSkillsShShortcut(r.Address)
		if err != nil {
			return FetchResult{}, err
		}
		if sc != nil {
			locate := findSoleSkillDirectory
			if sc.name != "" {
				locate = func(root string) (string, error) { return findSkillDirectory(root, sc.name) }
			}
			return fetchGitVersion(ctx, sc.repoURL, r.RequestedRef, r.Revision, "", locate)
		}
	}
	if !strings.HasPrefix(r.Address, g.scheme) {
		return FetchResult{}, fmt.Errorf("unsupported address")
	}
	repo, sub := splitRepoSubpath(strings.TrimPrefix(r.Address, g.scheme))
	if r.Subpath != "" {
		sub = r.Subpath
	}
	return fetchGitVersion(ctx, "https://"+g.resolveHost()+"/"+repo+".git", r.RequestedRef, r.Revision, sub, nil)
}
func fetchGitVersion(ctx context.Context, url, ref, revision, sub string, locate func(string) (string, error)) (result FetchResult, err error) {
	root, err := os.MkdirTemp("", "skm-versioned-*")
	if err != nil {
		return result, err
	}
	canonical, canonicalErr := filepath.EvalSymlinks(root)
	if canonicalErr != nil {
		_ = os.RemoveAll(root)
		return result, canonicalErr
	}
	root = canonical
	defer func() {
		if err != nil {
			_ = os.RemoveAll(root)
		}
	}()
	run := func(args ...string) (string, error) {
		out, e := exec.CommandContext(ctx, "git", args...).CombinedOutput()
		if e != nil {
			return "", fmt.Errorf("git acquisition: %s: %w", strings.TrimSpace(string(out)), e)
		}
		return strings.TrimSpace(string(out)), nil
	}
	args := []string{"clone", "--depth", "1"}
	if ref != "" && revision == "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", url, root)
	if _, err = run(args...); err != nil {
		return result, err
	}
	if revision != "" {
		if len(revision) != 40 && len(revision) != 64 {
			return result, fmt.Errorf("revision must be a full immutable commit ID")
		}
		for _, c := range revision {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return result, fmt.Errorf("invalid commit ID")
			}
		}
		if _, err = run("-C", root, "fetch", "--depth", "1", "origin", revision); err != nil {
			return result, err
		}
		if _, err = run("-C", root, "checkout", "--detach", revision); err != nil {
			return result, err
		}
	}
	head, err := run("-C", root, "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	if revision != "" && !strings.EqualFold(head, revision) {
		return result, fmt.Errorf("fetched revision mismatch")
	}
	if locate != nil {
		selected, e := locate(root)
		if e != nil {
			return result, e
		}
		sub, err = filepath.Rel(root, selected)
		if err != nil {
			return result, err
		}
	}
	if sub == "" {
		sub = "."
	}
	if !filepath.IsLocal(sub) {
		return result, fmt.Errorf("unsafe source subpath")
	}
	child, err := filepath.EvalSymlinks(filepath.Join(root, sub))
	if err != nil {
		return result, err
	}
	rel, err := filepath.Rel(root, child)
	if err != nil || !filepath.IsLocal(rel) {
		return result, fmt.Errorf("source subpath escapes clone")
	}
	return FetchResult{Path: root, RepositoryAddress: url, RequestedRef: ref, ResolvedSubpath: sub, Revision: head}, nil
}
