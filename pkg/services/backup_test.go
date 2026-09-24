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

	result, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)
	require.Contains(t, result.Entries, "skills")
	// A backup is one minified JSON document, not a mirrored directory tree,
	// and it records entry roots only — never file content.
	require.FileExists(t, result.Path)
	doc, err := readBackupDocument(result.Path)
	require.NoError(t, err)
	require.Contains(t, doc.Roots, "skills/local/demo")
}

func TestCreateBackupFailsWhenNothingToBackUp(t *testing.T) {
	root := t.TempDir()
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.CreateBackup(context.Background(), "")
	require.Error(t, err)
}

func TestRestoreBackupReportsRemovedEntryAsFailedWithoutRecreatingContent(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/references/example.md", "ref body")

	backup, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(root, "skills/local/demo")))
	require.Nil(t, svc.FindEntry("demo"))

	restore, err := svc.RestoreBackup(context.Background(), backup.Path, false)
	require.NoError(t, err)
	require.False(t, restore.Success)
	require.Len(t, restore.Items, 1)
	require.Equal(t, "failed", restore.Items[0].Status)

	// Backup carries no file content, so restore never recreates a removed entry.
	require.Nil(t, svc.FindEntry("demo"))
	require.NoFileExists(t, filepath.Join(root, "skills/local/demo/references/example.md"))
}

func TestRestoreBackupReinstallsToPriorTargets(t *testing.T) {
	svc, target, _ := exportFixture(t)
	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	_, err := svc.Install(context.Background(), "demo", InstallOptions{})
	require.NoError(t, err)
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, *target))

	backup, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)
	_, err = svc.Uninstall(context.Background(), "demo", InstallOptions{})
	require.NoError(t, err)

	// The entry itself is untouched by uninstall; restore only reinstalls it.
	restore, err := svc.RestoreBackup(context.Background(), backup.Path, false)
	require.NoError(t, err)
	require.True(t, restore.Success)
	require.Equal(t, "restored", restore.Items[0].Status)
	require.NotEmpty(t, restore.Items[0].Results)

	restored := svc.FindEntry("demo")
	require.NotNil(t, restored)
	require.Equal(t, common.InstallInstalled, svc.Installer.State(restored, *target))
}

func TestRestoreBackupOfStillPresentEntryIsANoOpReinstall(t *testing.T) {
	svc, _, root := exportFixture(t)
	backup, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)

	restore, err := svc.RestoreBackup(context.Background(), backup.Path, false)
	require.NoError(t, err)
	require.True(t, restore.Success)
	require.Equal(t, "restored", restore.Items[0].Status)
	require.FileExists(t, filepath.Join(root, "skills/local/demo/SKILL.md"))
}

func TestRestoreBackupRejectsUnknownFile(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.RestoreBackup(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.json"), false)
	require.Error(t, err)
}

func TestCreateBackupWritesToTheGivenFile(t *testing.T) {
	svc, _, _ := exportFixture(t)
	dest := filepath.Join(t.TempDir(), "nested", "my-backup.json")

	result, err := svc.CreateBackup(context.Background(), dest)
	require.NoError(t, err)
	require.Equal(t, dest, result.Path)
	require.FileExists(t, dest)

	restore, err := svc.RestoreBackup(context.Background(), dest, false)
	require.NoError(t, err)
	require.Equal(t, dest, restore.Path)
	require.True(t, restore.Success)
}

func TestRestoreBackupDefaultsToLatest(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)

	restore, err := svc.RestoreBackup(context.Background(), "", false)
	require.NoError(t, err)
	require.True(t, restore.Success)
	require.NotNil(t, svc.FindEntry("demo"))
}

func TestRestoreBackupForceOverwritesAConflictingTargetPath(t *testing.T) {
	svc, target, _ := exportFixture(t)
	_, err := svc.Install(context.Background(), "demo", InstallOptions{})
	require.NoError(t, err)
	backup, err := svc.CreateBackup(context.Background(), "")
	require.NoError(t, err)
	_, err = svc.Uninstall(context.Background(), "demo", InstallOptions{})
	require.NoError(t, err)

	// A user file now occupies the install path: reinstalling must be
	// refused without --force and must succeed with it.
	conflict := filepath.Join(target.Path, "demo")
	require.NoError(t, os.WriteFile(conflict, []byte("user real file"), 0o644))

	restore, err := svc.RestoreBackup(context.Background(), backup.Path, false)
	require.NoError(t, err)
	require.Contains(t, restore.Items[0].Reason, "reinstalling to its prior targets failed")
	content, _ := os.ReadFile(conflict)
	require.Equal(t, "user real file", string(content))

	restore, err = svc.RestoreBackup(context.Background(), backup.Path, true)
	require.NoError(t, err)
	require.Empty(t, restore.Items[0].Reason)
	require.Equal(t, common.InstallInstalled, svc.Installer.State(svc.FindEntry("demo"), *target))
}
