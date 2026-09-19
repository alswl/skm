package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// emptyCmdFixture builds a repository + config dir with a target but no
// pre-existing entries, for apply tests that must not collide by name.
func emptyCmdFixture(t *testing.T) (root, cfgDir string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))

	cfgDir = filepath.Join(t.TempDir(), "cfg")
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	writeTestFile(t, cfgDir, "config.yaml",
		"targets:\n  - name: t\n    path: "+targetDir+"\n    accepts: [skill, command]\n    strategies:\n      skill: skill-symlink\n      command: command-adapter\n")
	return root, cfgDir
}

func extractPayloadCommand(t *testing.T, createOutput string) string {
	t.Helper()
	start := strings.Index(createOutput, "'")
	end := strings.LastIndex(createOutput, "'")
	require.True(t, start >= 0 && end > start, "unexpected share create output: %q", createOutput)
	return createOutput[start+1 : end]
}

func TestShareCreateEmitsApplyCommand(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	out, err := runCmd(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, out, "skm share apply 'skm1z")
}

func TestShareCreateJSONIncludesModeAndEntries(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	out, err := runCmd(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"mode":"content"`)
	require.Contains(t, out, `"name":"skill-a"`)
}

func TestShareCreateUpstreamRejectsLocalOnlyEntry(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "share", "create", "skill-a", "--upstream", "--root", root, "--config", cfgDir)
	require.Error(t, err)
}

func TestShareApplyRejectsMalformedPayload(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "share", "apply", "not-a-payload", "--root", root, "--config", cfgDir)
	require.Error(t, err)
}

func TestShareApplyImportsIntoFreshRepositoryAndReportsResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // hermetic: the merged-in built-in targets point into an empty home
	srcRoot, srcCfgDir := cmdFixture(t)
	create, err := runCmd(t, "share", "create", "skill-a", "--root", srcRoot, "--config", srcCfgDir)
	require.NoError(t, err)
	payload := extractPayloadCommand(t, create)

	freshRoot, freshCfgDir := emptyCmdFixture(t)
	out, err := runCmd(t, "share", "apply", payload, "--root", freshRoot, "--config", freshCfgDir)
	require.NoError(t, err)
	require.Contains(t, out, `"skill-a"`)
}

func TestShareApplySkipsSameNameCollision(t *testing.T) {
	srcRoot, srcCfgDir := cmdFixture(t)
	create, err := runCmd(t, "share", "create", "skill-a", "--root", srcRoot, "--config", srcCfgDir)
	require.NoError(t, err)
	payload := extractPayloadCommand(t, create)

	// The destination already has its own skill-a, so apply must skip it.
	destRoot, destCfgDir := cmdFixture(t)
	_, err = runCmd(t, "share", "apply", payload, "--root", destRoot, "--config", destCfgDir)
	require.Error(t, err)
}
