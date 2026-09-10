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
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nfor arg; do dest=$arg; done\ncp -R \"$SKM_GIT_FIXTURE/.\" \"$dest\"\n"), 0o755))
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
