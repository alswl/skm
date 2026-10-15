package engines

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestMigrateLegacyMovesForkAndRecord(t *testing.T) {
	root := t.TempDir()
	rel := "skills/local/demo"
	require.NoError(t, os.MkdirAll(filepath.Join(root, rel), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0600))
	legacy := filepath.Join(root, ".skm-sync")
	require.NoError(t, os.MkdirAll(filepath.Join(legacy, "entries", dal.SyncKey(rel), "upstream-baseline"), 0700))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(legacy, "format.json"), map[string]int{"schema_version": 1}))
	base := filepath.Join(legacy, "entries", dal.SyncKey(rel), "upstream-baseline")
	require.NoError(t, os.WriteFile(filepath.Join(base, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0600))
	digest, err := dal.TreeFingerprint(base)
	require.NoError(t, err)
	oldFork := filepath.Join("forks", dal.SyncKey(rel))
	require.NoError(t, os.MkdirAll(filepath.Join(root, oldFork), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, oldFork, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\ncustom"), 0600))
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: "demo", Generation: 1, Active: "fork", ForkPath: oldFork, UpstreamVersion: "sha256:" + digest, ForkVersion: "sha256:" + digest, UpstreamDigest: digest, ForkDigest: digest}
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(legacy, "entries", dal.SyncKey(rel), "record.json"), rec))

	repo := NewRepository(root)
	preview, err := repo.MigrateLegacy(false, "", false)
	require.NoError(t, err)
	require.Equal(t, "planned", preview.Outcome)
	result, err := repo.MigrateLegacy(true, preview.ExpectedState, false)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.NoDirExists(t, legacy)
	require.DirExists(t, filepath.Join(root, dal.SyncForkRel(rel)))
	got, err := dal.ReadSyncRecord(root, rel)
	require.NoError(t, err)
	require.Equal(t, 2, got.Schema)
	require.Equal(t, dal.SyncForkRel(rel), got.ForkPath)
	require.Equal(t, "snapshot", got.UpstreamBase.Kind)
	require.DirExists(t, filepath.Join(root, ".skm", "baselines", digest))
	require.NoDirExists(t, filepath.Join(root, dal.SyncStateRel(rel), "upstream-baseline"))
	status, err := repo.SyncStatus(&common.Entry{Name: "demo", Kind: common.KindSkill, Path: filepath.Join(root, got.ForkPath), RepositoryPath: filepath.Join(root, rel)})
	require.NoError(t, err)
	require.Equal(t, "valid", status.BaselineState)
}

func TestMigrateLegacyRejectsInvalidFormatBeforeWriting(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".skm-sync")
	require.NoError(t, os.MkdirAll(filepath.Join(legacy, "entries"), 0700))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(legacy, "format.json"), map[string]int{"schema_version": 7}))
	_, err := NewRepository(root).MigrateLegacy(false, "", false)
	require.ErrorContains(t, err, "unsupported legacy format")
	require.NoDirExists(t, filepath.Join(root, ".skm"))
}
