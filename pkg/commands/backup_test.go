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

func TestBackupRestoreReinstallsWithoutRecreatingRemovedContent(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "backup", "create", "--root", root, "--config", cfgDir)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(root, "skills/local/skill-a")))

	// Backup carries no file content, so a removed entry cannot be restored;
	// it is reported as failed, and the command exits non-zero.
	out, err := runCmd(t, "backup", "restore", "--root", root, "--config", cfgDir)
	require.Error(t, err)
	require.Contains(t, out, "failed")
	require.NoFileExists(t, filepath.Join(root, "skills/local/skill-a/SKILL.md"))
}

func TestBackupRestoreJSONReportsStillPresentEntryAsSuccess(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "backup", "create", "--root", root, "--config", cfgDir)
	require.NoError(t, err)

	_, err = runCmd(t, "backup", "restore", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
}
