package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Pasting the URL from the browser address bar is how anyone actually refers
// to a skill that lives in a subdirectory of a repository. Handing that URL
// straight to `git clone` fails ("... is not a valid repository name"), so the
// provider has to recognise GitHub/GitLab *web* URLs and take them apart into
// the repository to clone, the ref to clone at, and the directory to stage.

func TestParseGitWebURL(t *testing.T) {
	cases := []struct {
		name    string
		address string
		host    string
		want    gitWebLocation
		ok      bool
	}{
		{
			name:    "blob URL for a file stages its directory",
			address: "https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md",
			host:    "github.com",
			want:    gitWebLocation{repoPath: "alswl/mind-forge", ref: "master", subdir: "skills/mf-cli"},
			ok:      true,
		},
		{
			name:    "tree URL already names the directory",
			address: "https://github.com/alswl/mind-forge/tree/master/skills/mf-cli",
			host:    "github.com",
			want:    gitWebLocation{repoPath: "alswl/mind-forge", ref: "master", subdir: "skills/mf-cli"},
			ok:      true,
		},
		{
			name:    "raw URL behaves like blob",
			address: "https://github.com/o/r/raw/v1.2.0/a/b/SKILL.md",
			host:    "github.com",
			want:    gitWebLocation{repoPath: "o/r", ref: "v1.2.0", subdir: "a/b"},
			ok:      true,
		},
		{
			name:    "tree URL at the repository root has no subdirectory",
			address: "https://github.com/o/r/tree/develop",
			host:    "github.com",
			want:    gitWebLocation{repoPath: "o/r", ref: "develop", subdir: ""},
			ok:      true,
		},
		{
			name:    "a file at the repository root has no subdirectory",
			address: "https://github.com/o/r/blob/main/SKILL.md",
			host:    "github.com",
			want:    gitWebLocation{repoPath: "o/r", ref: "main", subdir: ""},
			ok:      true,
		},
		{
			name:    "GitLab puts /-/ before blob",
			address: "https://gitlab.com/group/proj/-/blob/main/skills/x/SKILL.md",
			host:    "gitlab.com",
			want:    gitWebLocation{repoPath: "group/proj", ref: "main", subdir: "skills/x"},
			ok:      true,
		},
		{name: "plain clone URL is not a web URL", address: "https://github.com/o/r", host: "github.com"},
		{name: "clone URL with .git is not a web URL", address: "https://github.com/o/r.git", host: "github.com"},
		{name: "another host is not ours", address: "https://gitlab.com/o/r/blob/main/x/SKILL.md", host: "github.com"},
		{name: "SCP-style is not a web URL", address: "git@github.com:o/r.git", host: "github.com"},
		{name: "owner/repo shorthand is not a web URL", address: "o/r", host: "github.com"},
		{name: "unknown path kind is left alone", address: "https://github.com/o/r/releases/tag/v1", host: "github.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseGitWebURL(tc.address, tc.host)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.want, got)
			}
		})
	}
}

// A web URL must still land under <provider>/<owner>/<repo>/, exactly like the
// clone URL for the same repository — otherwise the same skill groups
// differently depending on which form of the address was pasted.
func TestGroupFromWebURL(t *testing.T) {
	gh := NewGitHub().(gitHostProvider)
	require.Equal(t, "alswl/mind-forge",
		gh.Group("https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md"))
	require.Equal(t, "alswl/mind-forge",
		gh.Group("https://github.com/alswl/mind-forge/tree/master/skills/mf-cli"))

	gl := NewGitLab().(gitHostProvider)
	require.Equal(t, "group/proj",
		gl.Group("https://gitlab.com/group/proj/-/blob/main/skills/x/SKILL.md"))
}

// CanHandle already accepted these (they are https URLs on the right host);
// the point of the test is that it keeps doing so, since that is what routes
// them to this provider at all.
func TestCanHandleWebURL(t *testing.T) {
	gh := NewGitHub().(gitHostProvider)
	require.True(t, gh.CanHandle("https://github.com/alswl/mind-forge/blob/master/skills/mf-cli/SKILL.md"))
	require.False(t, gh.CanHandle("https://gitlab.com/group/proj/-/blob/main/skills/x/SKILL.md"))
}

// "owner/repo/sub/dir" extends the existing exact "owner/repo" shorthand with
// a path inside the repository — the same convenience blob/tree URLs already
// give you, without having to paste a full URL. It is GitHub-only
// (allowSubpathShorthand): a GitHub repository path is always exactly
// "owner/repo", so segment three onward can never be anything but a
// subdirectory. GitLab nests groups ("group/subgroup/project" is itself a
// valid, two-*segment*-looking repository path once you count subgroups), so
// the same rule there would misparse a real repository as owner+subdir; a
// GitLab user who wants a subdirectory already has the browse-URL form.
func TestParseOwnerRepoSubpath(t *testing.T) {
	cases := []struct {
		name     string
		address  string
		repoPath string
		subdir   string
		ok       bool
	}{
		{name: "three segments", address: "alswl/mind-forge/skills", repoPath: "alswl/mind-forge", subdir: "skills", ok: true},
		{name: "deeper path", address: "alswl/mind-forge/skills/mf-cli", repoPath: "alswl/mind-forge", subdir: "skills/mf-cli", ok: true},
		{name: "exact owner/repo is not a subpath", address: "alswl/mind-forge", ok: false},
		{name: "empty segment is rejected", address: "alswl/mind-forge//mf-cli", ok: false},
		{name: "trailing slash is rejected", address: "alswl/mind-forge/skills/", ok: false},
		{name: "whitespace is rejected", address: "alswl/mind forge/skills", ok: false},
		{name: "a URL is not shorthand", address: "https://github.com/o/r/skills", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoPath, subdir, ok := parseOwnerRepoSubpath(tc.address)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.repoPath, repoPath)
				require.Equal(t, tc.subdir, subdir)
			}
		})
	}
}

func TestGitHubAcceptsSubpathShorthand(t *testing.T) {
	gh := NewGitHub().(gitHostProvider)
	require.True(t, gh.CanHandle("alswl/mind-forge/skills/mf-cli"))
	require.Equal(t, "alswl/mind-forge", gh.Group("alswl/mind-forge/skills/mf-cli"))
}

// GitLab must not gain the same shorthand: "group/subgroup/project" is a real,
// two-segment-looking repository path, and treating segment three as a
// subdirectory would silently clone the wrong thing.
func TestGitLabDoesNotAcceptSubpathShorthand(t *testing.T) {
	gl := NewGitLab().(gitHostProvider)
	require.False(t, gl.CanHandle("group/subgroup/project"),
		"a three-segment address must not be claimed as owner/repo/subdir — it may be a real nested-group repository")
}
