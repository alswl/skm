package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestCreateBackupCapturesEntriesAndMetadata(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/references/example.md", "ref body")

	result, err := svc.CreateBackup(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, result.ID)
	require.Contains(t, result.Entries, "skills")
	require.FileExists(t, filepath.Join(result.Path, "skills/local/demo/SKILL.md"))
	require.FileExists(t, filepath.Join(result.Path, "skills/local/demo/references/example.md"))
	require.FileExists(t, filepath.Join(result.Path, backupMetaFile))
}

func TestCreateBackupFailsWhenNothingToBackUp(t *testing.T) {
	root := t.TempDir()
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.CreateBackup(context.Background())
	require.Error(t, err)
}

func TestRestoreBackupRoundTripRecoversRemovedEntry(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/references/example.md", "ref body")

	backup, err := svc.CreateBackup(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(root, "skills/local/demo")))
	require.Nil(t, svc.FindEntry("demo"))

	restore, err := svc.RestoreBackup(context.Background(), backup.ID)
	require.NoError(t, err)
	require.True(t, restore.Success)
	require.Len(t, restore.Items, 1)
	require.Equal(t, "restored", restore.Items[0].Status)

	require.NotNil(t, svc.FindEntry("demo"))
	require.FileExists(t, filepath.Join(root, "skills/local/demo/references/example.md"))
}

func TestRestoreBackupSkipsExistingEntryWithoutOverwrite(t *testing.T) {
	svc, _, root := exportFixture(t)
	backup, err := svc.CreateBackup(context.Background())
	require.NoError(t, err)

	restore, err := svc.RestoreBackup(context.Background(), backup.ID)
	require.NoError(t, err)
	require.False(t, restore.Success)
	require.Equal(t, "skipped", restore.Items[0].Status)
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))
}

func TestRestoreBackupRejectsUnknownID(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.RestoreBackup(context.Background(), "does-not-exist")
	require.Error(t, err)
}

func TestRestoreBackupDefaultsToLatest(t *testing.T) {
	svc, _, root := exportFixture(t)
	_, err := svc.CreateBackup(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "skills/local/demo")))

	restore, err := svc.RestoreBackup(context.Background(), "")
	require.NoError(t, err)
	require.True(t, restore.Success)
	require.NotNil(t, svc.FindEntry("demo"))
}
