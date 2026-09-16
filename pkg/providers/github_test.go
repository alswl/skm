package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeGitFixture(t *testing.T) string {
	t.Helper()
	fixture := t.TempDir()
	bin := t.TempDir()
	script := filepath.Join(bin, "git")
	// The shim stands in for the system git: a clone copies the fixture into
	// its destination directory, a sparse-checkout is a no-op success.
	// SKM_GIT_FAIL_ON_FILTER simulates a git/host without partial-clone
	// support so the sparse-clone fallback can be exercised offline; every
	// invocation's args are appended to SKM_GIT_ARGS_LOG when set.
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
[ -n "$SKM_GIT_ARGS_LOG" ] && echo "$@" >> "$SKM_GIT_ARGS_LOG"
for arg; do
	case $arg in
		sparse-checkout) exit 0 ;;
		--filter=blob:none) [ -n "$SKM_GIT_FAIL_ON_FILTER" ] && exit 1 ;;
	esac
done
for arg; do dest=$arg; done
cp -R "$SKM_GIT_FIXTURE/." "$dest"
`), 0o755))
	t.Setenv("SKM_GIT_FIXTURE", fixture)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fixture
}

func TestGitHubBrowseURLStagesContainingSkillDirectory(t *testing.T) {
	fixture := fakeGitFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fixture, "skills", "mf-cli"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture, "skills", "mf-cli", "SKILL.md"), []byte("---\nname: mf-cli\n---\n"), 0o644))
	g := NewGitHub().(gitHostProvider)
	staged, err := g.Fetch(context.Background(), "https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(staged) })
	require.FileExists(t, filepath.Join(staged, "SKILL.md"))
}

func TestGitHubBrowseURLMissingDirectoryNamesRepositoryRefAndSubdirectory(t *testing.T) {
	fakeGitFixture(t)
	g := NewGitHub().(gitHostProvider)
	_, err := g.Fetch(context.Background(), "https://github.com/alswl/mind-forge/tree/master/skills/missing")
	require.Error(t, err)
	require.Contains(t, err.Error(), "alswl/mind-forge")
	require.Contains(t, err.Error(), "master")
	require.Contains(t, err.Error(), "skills/missing")
}

// A browse URL stages the named subdirectory through a partial (filter+blob:
// none, sparse) clone, so a skill inside a huge repository costs that
// directory's bytes instead of the whole repository's — the difference between
// seconds and minutes for the import the user is waiting on.
func TestGitHubBrowseURLClonesSparseWhenStagingSubdirectory(t *testing.T) {
	fixture := fakeGitFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fixture, "skills", "mf-cli"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture, "skills", "mf-cli", "SKILL.md"), []byte("---\nname: mf-cli\n---\n"), 0o644))
	log := filepath.Join(t.TempDir(), "git-args.log")
	t.Setenv("SKM_GIT_ARGS_LOG", log)
	g := NewGitHub().(gitHostProvider)
	staged, err := g.Fetch(context.Background(), "https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(staged) })
	require.FileExists(t, filepath.Join(staged, "SKILL.md"))
	args, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Contains(t, string(args), "--filter=blob:none")
	require.Contains(t, string(args), "--sparse")
	require.Contains(t, string(args), "sparse-checkout set skills/mf-cli")
}

// An old git, or a host without filter support, must not break the import:
// the sparse attempt falls back to the plain shallow clone.
func TestGitHubBrowseURLFallsBackToFullShallowCloneWithoutFilterSupport(t *testing.T) {
	fixture := fakeGitFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fixture, "skills", "mf-cli"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture, "skills", "mf-cli", "SKILL.md"), []byte("---\nname: mf-cli\n---\n"), 0o644))
	t.Setenv("SKM_GIT_FAIL_ON_FILTER", "1")
	log := filepath.Join(t.TempDir(), "git-args.log")
	t.Setenv("SKM_GIT_ARGS_LOG", log)
	g := NewGitHub().(gitHostProvider)
	staged, err := g.Fetch(context.Background(), "https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(staged) })
	require.FileExists(t, filepath.Join(staged, "SKILL.md"))
	args, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Contains(t, string(args), "--filter=blob:none") // the sparse attempt happened…
	require.NotContains(t, string(args), "sparse-checkout") // …but the fallback clone ran without one
}

// TestGitHubProviderGroupDerivesOwnerRepo covers every address form CanHandle
// accepts (owner/repo shorthand, https, ssh://, git@ SCP-style, a bare .git
// URL) plus the forms it must reject (wrong host, not exactly two segments).
func TestGitHubProviderGroupDerivesOwnerRepo(t *testing.T) {
	p := NewGitHub().(gitHostProvider)
	cases := []struct {
		name, address, want string
	}{
		{"owner/repo shorthand", "octocat/hello-world", "octocat/hello-world"},
		{"https URL", "https://github.com/octocat/hello-world", "octocat/hello-world"},
		{"https URL with .git suffix", "https://github.com/octocat/hello-world.git", "octocat/hello-world"},
		{"ssh URL", "ssh://git@github.com/octocat/hello-world.git", "octocat/hello-world"},
		{"git@ SCP-style", "git@github.com:octocat/hello-world.git", "octocat/hello-world"},
		{"wrong host", "https://gitlab.com/octocat/hello-world", ""},
		{"not two segments", "https://github.com/octocat/hello-world/extra", ""},
		{"unrelated string", "not a url at all", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, p.Group(c.address))
		})
	}
}

// TestGitLabProviderGroupUsesItsOwnHost: GitLab's Group must only match
// gitlab.com addresses, not github.com ones (each gitHostProvider instance is
// scoped to its own host, same as CanHandle).
func TestGitLabProviderGroupUsesItsOwnHost(t *testing.T) {
	p := NewGitLab().(gitHostProvider)
	require.Equal(t, "acme/widgets", p.Group("https://gitlab.com/acme/widgets"))
	require.Equal(t, "", p.Group("https://github.com/acme/widgets"), "a github.com address is not this provider's host")
}
