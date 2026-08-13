package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// GitLab shares GitHub's access pattern (git URLs + owner/repo shorthand,
// https clone); skills.sh keeps a scheme-prefixed address that resolves to
// github.com (skills.sh indexes GitHub repos).

func TestBuiltinProvidersCanHandleTheirOwnScheme(t *testing.T) {
	cases := []struct {
		provider Provider
		match    string
		nomatch  string
	}{
		{NewGitLab(), "team/skills-repo", "skills.sh://team/skills-repo"},
		{NewGitLab(), "https://gitlab.com/team/skills-repo.git", "skills.sh://x/y"},
		{NewSkillsSh(), "skills.sh://owner/skills-repo", "owner/skills-repo"},
	}
	for _, c := range cases {
		require.True(t, c.provider.CanHandle(c.match), "%s should handle %s", c.provider.ID(), c.match)
		require.False(t, c.provider.CanHandle(c.nomatch), "%s should not handle %s", c.provider.ID(), c.nomatch)
	}
}

// TestGitHostProvidersDoNotClaimOtherHosts: a git URL for a host that isn't
// github.com/gitlab.com (e.g. an internal git server) must not be claimed by
// GitHub or GitLab just because it's git-shaped — otherwise it's grabbed
// before a plugin provider registered for that host ever sees it, and the
// plugin's host-specific auth/fetch handling is silently skipped (the built-in
// provider's plain `git clone` then fails for hosts that need it).
func TestGitHostProvidersDoNotClaimOtherHosts(t *testing.T) {
	addr := "https://git.example.com/team/skills-repo.git"
	require.False(t, NewGitHub().CanHandle(addr))
	require.False(t, NewGitLab().CanHandle(addr))

	// Still claims their own host and owner/repo shorthand.
	require.True(t, NewGitHub().CanHandle("https://github.com/team/skills-repo.git"))
	require.True(t, NewGitHub().CanHandle("owner/repo"))
	require.True(t, NewGitLab().CanHandle("https://gitlab.com/team/skills-repo.git"))
}

func TestBuiltinProvidersNormalizeToCanonicalGitURL(t *testing.T) {
	// GitLab resolves owner/repo shorthand to its own https host, exactly like
	// GitHub does for github.com (same access pattern, different host).
	url, err := NewGitLab().Normalize("team/skills-repo")
	require.NoError(t, err)
	require.Equal(t, "https://gitlab.com/team/skills-repo", url)

	url, err = NewGitHub().Normalize("team/skills-repo")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/team/skills-repo", url)

	// A real gitlab URL passes through unchanged.
	url, err = NewGitLab().Normalize("https://gitlab.com/team/skills-repo.git")
	require.NoError(t, err)
	require.Equal(t, "https://gitlab.com/team/skills-repo.git", url)
}

// skills.sh's Normalize is a no-op (see gitBackedProvider.Normalize):
// fetchProvider calls Fetch with Normalize's result, so rewriting the
// scheme away here would leave Fetch nothing it can claim or parse.
func TestSkillsShNormalizeIsANoOp(t *testing.T) {
	url, err := NewSkillsSh().Normalize("skills.sh://owner/skills-repo")
	require.NoError(t, err)
	require.Equal(t, "skills.sh://owner/skills-repo", url)
}

func TestBuiltinProviderNormalizeUnrelatedAddressIsUnchanged(t *testing.T) {
	addr := "https://github.com/foo/bar"
	got, err := NewSkillsSh().Normalize(addr)
	require.NoError(t, err)
	require.Equal(t, addr, got, "an address outside this provider's scheme is returned unchanged, not an error")
}

func TestBuiltinProviderFetchRejectsMismatchedAddress(t *testing.T) {
	_, err := NewSkillsSh().Fetch(t.Context(), "gitlab://foo")
	require.Error(t, err)
	var pe *ProviderError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, CodeUnsupportedAddress, pe.Code)
}

// skills.sh addresses are owner/repo paths by construction (it indexes skills
// living in public GitHub repos), so a third path segment can only be a
// subdirectory — the same shape GitHub's owner/repo/subdir shorthand accepts,
// and for the same reason: no subgroup nesting to be ambiguous with. Fetch
// (not Normalize) is what resolves the subpath, so Normalize must keep it
// intact for Fetch to see.
func TestSkillsShNormalizeKeepsSubpath(t *testing.T) {
	url, err := NewSkillsSh().Normalize("skills.sh://alswl/mind-forge/skills/mf-cli")
	require.NoError(t, err)
	require.Equal(t, "skills.sh://alswl/mind-forge/skills/mf-cli", url)
}

func TestSkillsShCanHandleSubpathAddress(t *testing.T) {
	require.True(t, NewSkillsSh().CanHandle("skills.sh://alswl/mind-forge/skills/mf-cli"))
}

// skills.sh's own site shows two copy-paste forms — its npx install command
// and a skill's page URL — that name a skill by its short directory name,
// not its "skills.sh://" scheme. Both must be recognized directly.
func TestSkillsShCanHandleCopyPasteForms(t *testing.T) {
	require.True(t, NewSkillsSh().CanHandle("npx skills add https://github.com/vercel-labs/skills --skill find-skills"))
	require.True(t, NewSkillsSh().CanHandle("$ npx skills add https://github.com/mattpocock/skills --skill grill-me"))
	require.True(t, NewSkillsSh().CanHandle("https://skills.sh/vercel-labs/skills/find-skills"))
}

func TestSplitRepoSubpath(t *testing.T) {
	cases := []struct {
		path, repoPath, subdir string
	}{
		{"owner/repo", "owner/repo", ""},
		{"owner/repo/skills", "owner/repo", "skills"},
		{"owner/repo/skills/mf-cli", "owner/repo", "skills/mf-cli"},
		{"owner", "owner", ""}, // too short to split; passed through as-is
		// A trailing slash must not survive into the repo path: it would build
		// "https://github.com/owner/repo/.git", which git rejects.
		{"owner/repo/", "owner/repo", ""},
		{"/owner/repo/", "owner/repo", ""},
	}
	for _, tc := range cases {
		repoPath, subdir := splitRepoSubpath(tc.path)
		require.Equal(t, tc.repoPath, repoPath, "repoPath for %q", tc.path)
		require.Equal(t, tc.subdir, subdir, "subdir for %q", tc.path)
	}
}

func TestBuiltinProvidersHaveDistinctIDsAndCapabilities(t *testing.T) {
	for _, p := range []Provider{NewGitHub(), NewGitLab(), NewSkillsSh()} {
		cap := p.Capability()
		require.Equal(t, p.ID(), cap.ID)
		require.NotEmpty(t, cap.Description)
		require.NotEmpty(t, cap.Schemes)
	}
}
