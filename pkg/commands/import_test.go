package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportCommandLocalJSON(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "review")
	writeTestFile(t, src, "SKILL.md", "---\nname: review\ndescription: a review skill\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	out, err := runCmd(t, "import", src, "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"name":"review"`)
	require.Contains(t, out, `"type":"skill"`)
	require.Contains(t, out, `"provider":"local"`)
	require.Contains(t, out, `"origin":{"address":"`+src+`","mode_id":"local","path":"skills/local/review"}`)
	// Placement on disk.
	require.FileExists(t, filepath.Join(root, "skills", "local", "review", "SKILL.md"))
	// The source dir must remain (copied, not moved).
	require.FileExists(t, filepath.Join(src, "SKILL.md"))
}

func TestImportCommandDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "review")
	writeTestFile(t, src, "SKILL.md", "---\nname: review\ndescription: a review skill\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	_, err := runCmd(t, "import", src, "--root", root, "--config", cfgDir, "--json", "--dry-run")
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(root, "skills", "local", "review"))
	require.True(t, os.IsNotExist(statErr), "dry-run must not place the entry")
}

func TestImportCommandCollisionRefused(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/local/dup/SKILL.md", "---\nname: dup\ndescription: first\n---\nbody\n")
	src := filepath.Join(t.TempDir(), "dup")
	writeTestFile(t, src, "SKILL.md", "---\nname: dup\ndescription: second\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	_, err := runCmd(t, "import", src, "--root", root, "--config", cfgDir)
	require.Error(t, err, "collision must be refused without --force")
}

func TestImportCommandClaimsMalformedSkillDirectory(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(t.TempDir(), "cfg")
	src := filepath.Join(root, "skills", "legacy")
	writeTestFile(t, src, "SKILL.md", "---\ndescription: old skill\n---\nbody\n")

	out, err := runCmd(t, "import", src, "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"legacy","type":"skill","provider":"local","path":"`+filepath.Join(root, "skills", "local", "legacy")+`","origin":{"address":"`+src+`","mode_id":"local","path":"skills/local/legacy"}}`, out)
	require.FileExists(t, filepath.Join(root, "skills", "local", "legacy", "SKILL.md"))
	require.NoDirExists(t, src)
}

// runCmdStdin runs a command with stdin bound to text, restoring the default
// afterwards so later tests are unaffected.
func runCmdStdin(t *testing.T, text string, args ...string) (string, error) {
	t.Helper()
	rootCmd.SetIn(strings.NewReader(text))
	t.Cleanup(func() { rootCmd.SetIn(os.Stdin) })
	return runCmd(t, args...)
}

func TestImportCommandReadsSourceListFromStdin(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	alpha := filepath.Join(srcDir, "alpha")
	beta := filepath.Join(srcDir, "beta")
	writeTestFile(t, alpha, "SKILL.md", "---\nname: alpha\ndescription: a\n---\nbody\n")
	writeTestFile(t, beta, "SKILL.md", "---\nname: beta\ndescription: b\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	stdin := alpha + "\n\n# a comment\n" + beta + "\n"
	out, err := runCmdStdin(t, stdin, "import", "-", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	// One JSON array holding both, in stdin order; blanks and comments skipped.
	require.Contains(t, out, `"name":"alpha"`)
	require.Contains(t, out, `"name":"beta"`)
	require.True(t, strings.HasPrefix(strings.TrimSpace(out), "["))
	require.FileExists(t, filepath.Join(root, "skills", "local", "alpha", "SKILL.md"))
	require.FileExists(t, filepath.Join(root, "skills", "local", "beta", "SKILL.md"))
}

func TestImportCommandStdinReportsSuccessesBeforeAFailure(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(t.TempDir(), "alpha")
	writeTestFile(t, alpha, "SKILL.md", "---\nname: alpha\ndescription: a\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	out, err := runCmdStdin(t, alpha+"\n/nope/does/not/exist\n", "import", "-", "--root", root, "--config", cfgDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "/nope/does/not/exist", "the failing source is named")
	require.Contains(t, out, "imported alpha", "what already landed on disk is still reported")
	require.FileExists(t, filepath.Join(root, "skills", "local", "alpha", "SKILL.md"))
}

func TestImportCommandStdinRejectsAnEmptyList(t *testing.T) {
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")
	_, err := runCmdStdin(t, "\n#only a comment\n", "import", "-", "--root", t.TempDir(), "--config", cfgDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no sources")
}
