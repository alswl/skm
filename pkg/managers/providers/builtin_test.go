package providers

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

	// skills.sh indexes GitHub repos, so the scheme resolves to github.com.
	url, err = NewSkillsSh().Normalize("skills.sh://owner/skills-repo")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/skills-repo.git", url)
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

func TestBuiltinProvidersHaveDistinctIDsAndCapabilities(t *testing.T) {
	for _, p := range []Provider{NewGitHub(), NewGitLab(), NewSkillsSh()} {
		cap := p.Capability()
		require.Equal(t, p.ID(), cap.ID)
		require.NotEmpty(t, cap.Description)
		require.NotEmpty(t, cap.Schemes)
	}
}
