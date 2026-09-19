package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupCreateReportsIDAndPath(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	out, err := runCmd(t, "backup", "create", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, out, "created backup")
}

func TestBackupRestoreRoundTripAfterRemoval(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "backup", "create", "--root", root, "--config", cfgDir)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(root, "skills/local/skill-a")))

	out, err := runCmd(t, "backup", "restore", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, out, "restored")
	require.FileExists(t, filepath.Join(root, "skills/local/skill-a/SKILL.md"))
}

func TestBackupRestoreJSONReportsConflictAsNonSuccess(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "backup", "create", "--root", root, "--config", cfgDir)
	require.NoError(t, err)

	_, err = runCmd(t, "backup", "restore", "--root", root, "--config", cfgDir, "--json")
	require.Error(t, err)
}
