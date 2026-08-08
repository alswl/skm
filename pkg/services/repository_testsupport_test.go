package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFile creates a file under root with parents.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// frontmatter renders a marker body with name/description/version.
func frontmatter(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\nbody\n"
}

// buildFixtureRepo creates a repository with a mixed set of entries:
//
//	skills/local/skill-a/SKILL.md          valid skill
//	skills/local/team/skill-b/SKILL.md     grouped skill
//	commands/github/cmd-a/command.md       directory command (remote origin)
//	commands/local/single.md               single-file command (stem name)
//	archived/local/old-skill/SKILL.md      archived skill
//	skills/local/bad-marker/SKILL.md       no frontmatter name -> error
func buildFixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "skills/local/skill-a/SKILL.md", frontmatter("skill-a", "A skill"))
	writeFile(t, root, "skills/local/team/skill-b/SKILL.md", frontmatter("skill-b", "A grouped skill"))
	writeFile(t, root, "commands/github/cmd-a/command.md", frontmatter("cmd-a", "A remote command"))
	writeFile(t, root, "commands/github/cmd-a/meta.json", `{"address":"https://github.com/x/y","mode_id":"github"}`)
	writeFile(t, root, "commands/local/single.md", "---\ndescription: no name here\n---\nbody\n")
	writeFile(t, root, "archived/local/old-skill/SKILL.md", frontmatter("old-skill", "An archived skill"))
	writeFile(t, root, "skills/local/bad-marker/SKILL.md", "---\ndescription: missing name\n---\nbody\n")
	return root
}
