package dal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestSyncStoreRejectsForeignAndCorruptState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".skm-local"), 0700))
	require.Error(t, InitSyncStore(root))
	root = t.TempDir()
	require.NoError(t, InitSyncStore(root))
	require.NoError(t, InitSyncStore(root))
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, "/.skm-local/\n", string(b))
	rel := "skills/local/demo"
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: "demo", Generation: 1, Active: "fork", ForkPath: "../escape"}
	require.NoError(t, WriteSyncJSON(filepath.Join(root, SyncStateRel(rel), "record.json"), rec))
	_, err = ReadSyncRecord(root, rel)
	require.Error(t, err)
	rec.ForkPath = filepath.Join("forks", SyncKey(rel))
	rec.Schema = 99
	require.NoError(t, WriteSyncJSON(filepath.Join(root, SyncStateRel(rel), "record.json"), rec))
	_, err = ReadSyncRecord(root, rel)
	require.Error(t, err)
}
