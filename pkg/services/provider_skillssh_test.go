package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSkillsShShortcutNpxCommand(t *testing.T) {
	sc := parseSkillsShShortcut("npx skills add https://github.com/vercel-labs/skills --skill find-skills")
	require.NotNil(t, sc)
	require.Equal(t, "https://github.com/vercel-labs/skills.git", sc.repoURL)
	require.Equal(t, "find-skills", sc.name)

	// The shell prompt is often copied along with the command.
	sc = parseSkillsShShortcut("$ npx skills add https://github.com/mattpocock/skills --skill grill-me")
	require.NotNil(t, sc)
	require.Equal(t, "https://github.com/mattpocock/skills.git", sc.repoURL)
	require.Equal(t, "grill-me", sc.name)

	// A URL that already ends in .git must not gain a second one.
	sc = parseSkillsShShortcut("npx skills add https://github.com/owner/repo.git --skill name")
	require.NotNil(t, sc)
	require.Equal(t, "https://github.com/owner/repo.git", sc.repoURL)
}

func TestParseSkillsShShortcutPageURL(t *testing.T) {
	sc := parseSkillsShShortcut("https://skills.sh/vercel-labs/skills/find-skills")
	require.NotNil(t, sc)
	require.Equal(t, "https://github.com/vercel-labs/skills.git", sc.repoURL)
	require.Equal(t, "find-skills", sc.name)
}

func TestParseSkillsShShortcutPageURLAcceptsWWW(t *testing.T) {
	sc := parseSkillsShShortcut("https://www.skills.sh/mattpocock/skills/improve-codebase-architecture")
	require.NotNil(t, sc)
	require.Equal(t, "https://github.com/mattpocock/skills.git", sc.repoURL)
	require.Equal(t, "improve-codebase-architecture", sc.name)
}

func TestParseSkillsShShortcutRejectsUnrelatedAddresses(t *testing.T) {
	for _, addr := range []string{
		"skills.sh://owner/repo",
		"owner/repo",
		"https://github.com/owner/repo",
		"npx skills add https://github.com/owner/repo", // missing --skill
	} {
		require.Nil(t, parseSkillsShShortcut(addr), "address %q must not match", addr)
	}
}

func TestFindSkillDirectoryLocatesByNameAndMarker(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "productivity", "grill-me")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: grill-me\n---\n"), 0o644))

	// A same-named directory with no marker must not count as a match.
	decoy := filepath.Join(root, "docs", "grill-me")
	require.NoError(t, os.MkdirAll(decoy, 0o755))

	got, err := findSkillDirectory(root, "grill-me")
	require.NoError(t, err)
	require.Equal(t, skillDir, got)
}

func TestFindSkillDirectoryNoMatch(t *testing.T) {
	root := t.TempDir()
	_, err := findSkillDirectory(root, "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no skill or command directory named")
}

func TestFindSkillDirectoryAmbiguousMatch(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a/dup", "b/dup"} {
		p := filepath.Join(root, dir)
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("x"), 0o644))
	}
	_, err := findSkillDirectory(root, "dup")
	require.Error(t, err)
	require.Contains(t, err.Error(), "matches multiple directories")
}
