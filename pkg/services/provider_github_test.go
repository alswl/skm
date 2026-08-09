package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
