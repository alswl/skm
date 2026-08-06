package main

import (
	"os"
	"path/filepath"
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
	require.Contains(t, out, `"origin":null`)
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
