package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSkillsShShortcutNpxCommand(t *testing.T) {
	sc, err := parseSkillsShShortcut("npx skills add https://github.com/vercel-labs/skills --skill find-skills")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/vercel-labs/skills.git", sc.repoURL)
	require.Equal(t, "find-skills", sc.name)

	// The shell prompt is often copied along with the command.
	sc, err = parseSkillsShShortcut("$ npx skills add https://github.com/mattpocock/skills --skill grill-me")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/mattpocock/skills.git", sc.repoURL)
	require.Equal(t, "grill-me", sc.name)

	// A URL that already ends in .git must not gain a second one.
	sc, err = parseSkillsShShortcut("npx skills add https://github.com/owner/repo.git --skill name")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/repo.git", sc.repoURL)
}

func TestParseSkillsShShortcutOwnerRepoShorthand(t *testing.T) {
	// The bare form skills.sh docs lead with: no --skill, so no name — Fetch
	// resolves the repo's sole skill directory.
	sc, err := parseSkillsShShortcut("npx skills add ant-design/ant-design-cli")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/ant-design/ant-design-cli.git", sc.repoURL)
	require.Equal(t, "", sc.name)

	// Shorthand combines with --skill and the tool's other flags, and a
	// copied "$ " prompt must not break either form.
	sc, err = parseSkillsShShortcut("$ npx skills add ant-design/ant-design-cli --skill antd -g -y")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/ant-design/ant-design-cli.git", sc.repoURL)
	require.Equal(t, "antd", sc.name)
}

func TestParseSkillsShShortcutNpxWithoutSkillNamesSoleSkill(t *testing.T) {
	// --skill is optional for full URLs too, and flags the npx tool takes on
	// its own (-g, -y, --all, …) are ignored rather than misread.
	sc, err := parseSkillsShShortcut("npx skills add https://github.com/owner/repo -g -y --all --copy")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/repo.git", sc.repoURL)
	require.Equal(t, "", sc.name)
}

func TestParseSkillsShShortcutAcceptsShortSkillFlag(t *testing.T) {
	// -s is the npx tool's short form of --skill (the docs show both).
	sc, err := parseSkillsShShortcut("$ npx skills add mattpocock/skills -s grill-me -g")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/mattpocock/skills.git", sc.repoURL)
	require.Equal(t, "grill-me", sc.name)
}

func TestParseSkillsShShortcutRejectsSeveralSkillNames(t *testing.T) {
	// skm imports one skill at a time; --skill a b is the npx tool's
	// multi-select spelled out.
	_, err := parseSkillsShShortcut("npx skills add owner/repo --skill a b")
	require.Error(t, err)
	require.Contains(t, err.Error(), "several skills")
}

func TestParseSkillsShShortcutRejectsUnusableSource(t *testing.T) {
	_, err := parseSkillsShShortcut("npx skills add ./my-local-skills")
	require.Error(t, err)
	require.Contains(t, err.Error(), "neither owner/repo shorthand nor a git URL")
}

func TestParseSkillsShShortcutPageURL(t *testing.T) {
	sc, err := parseSkillsShShortcut("https://skills.sh/vercel-labs/skills/find-skills")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/vercel-labs/skills.git", sc.repoURL)
	require.Equal(t, "find-skills", sc.name)
}

func TestParseSkillsShShortcutPageURLAcceptsWWW(t *testing.T) {
	sc, err := parseSkillsShShortcut("https://www.skills.sh/mattpocock/skills/improve-codebase-architecture")
	require.NoError(t, err)
	require.Equal(t, "https://github.com/mattpocock/skills.git", sc.repoURL)
	require.Equal(t, "improve-codebase-architecture", sc.name)
}

func TestParseSkillsShShortcutRejectsUnrelatedAddresses(t *testing.T) {
	for _, addr := range []string{
		"skills.sh://owner/repo",
		"owner/repo",
		"https://github.com/owner/repo",
		"npx install owner/repo",
	} {
		sc, err := parseSkillsShShortcut(addr)
		require.Nil(t, sc, "address %q must not match", addr)
		require.NoError(t, err, "address %q must not match", addr)
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

func writeSkillMarker(t *testing.T, root, dir string) {
	t.Helper()
	p := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(p, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644))
}

func TestFindSoleSkillDirectoryResolvesTheSingleSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "antd")
	writeSkillMarker(t, root, filepath.Join("skills", "antd"))

	got, err := findSoleSkillDirectory(root)
	require.NoError(t, err)
	require.Equal(t, skillDir, got)
}

func TestFindSoleSkillDirectoryNoSkill(t *testing.T) {
	root := t.TempDir()
	_, err := findSoleSkillDirectory(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no skill or command directory found")
}

func TestFindSoleSkillDirectoryListsSeveral(t *testing.T) {
	root := t.TempDir()
	writeSkillMarker(t, root, filepath.Join("skills", "a"))
	writeSkillMarker(t, root, filepath.Join("skills", "b"))

	_, err := findSoleSkillDirectory(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ships several skills")
	require.Contains(t, err.Error(), "skills/a, skills/b")
	require.Contains(t, err.Error(), "--skill <name>")
}
