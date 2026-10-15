package dal

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestSyncStoreRejectsForeignAndCorruptState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".skm"), 0700))
	require.Error(t, InitSyncStore(root))
	root = t.TempDir()
	require.NoError(t, InitSyncStore(root))
	require.NoError(t, InitSyncStore(root))
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, "!/.skm/\n!/.skm/format.json\n!/.skm/entries/\n!/.skm/entries/**\n/.skm/cache/\n/.skm/work/\n", string(b))
	rel := "skills/local/demo"
	rec := common.SyncRecord{Schema: 2, EntryPath: rel, Name: "demo", Generation: 1, Active: "fork", ForkPath: "../escape"}
	require.NoError(t, WriteSyncJSON(filepath.Join(root, SyncStateRel(rel), "record.json"), rec))
	_, err = ReadSyncRecord(root, rel)
	require.Error(t, err)
	rec.ForkPath = SyncForkRel(rel)
	rec.Schema = 99
	require.NoError(t, WriteSyncJSON(filepath.Join(root, SyncStateRel(rel), "record.json"), rec))
	_, err = ReadSyncRecord(root, rel)
	require.Error(t, err)
}

func TestSyncStoreDerivesOnlySafeBaselineAndCachePaths(t *testing.T) {
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	baseline, err := SyncBaselineRel(digest)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(".skm", "baselines", digest), baseline)
	cache, err := SyncCacheRel("HTTPS://Example.invalid/Owner/Repo.git", digest)
	require.NoError(t, err)
	require.True(t, filepath.IsLocal(cache))
	require.Contains(t, cache, filepath.Join(".skm", "cache", "repos"))
	_, err = SyncBaselineRel("../escape")
	require.Error(t, err)
	_, err = SyncCacheRel("repo", "branch")
	require.Error(t, err)
}

func TestSyncStoreRejectsSymlinkTrackedLocalStateAndReadInitialization(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".skm")))
	require.Error(t, InitSyncStore(root))
	require.NoFileExists(t, filepath.Join(outside, "format.json"))

	root = t.TempDir()
	_, err := ReadSyncRecord(root, "skills/local/demo")
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(root, ".skm"), "reads must not initialize management state")

	root = t.TempDir()
	run := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".skm", "cache"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".skm", "cache", "tracked"), []byte("x"), 0600))
	run("add", "-f", ".skm/cache/tracked")
	require.ErrorContains(t, InitSyncStore(root), "Git-tracked")
}

func TestReadSyncRecordValidatesV2BaselineReferences(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	rel := "skills/local/demo"
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	bad := common.BaselineRef{Kind: "git", Revision: "branch", RawDigest: digest}
	rec := common.SyncRecord{Schema: 2, EntryPath: rel, Name: "demo", Generation: 1, Active: "upstream", UpstreamBase: bad}
	require.NoError(t, WriteSyncJSON(filepath.Join(root, SyncStateRel(rel), "record.json"), rec))
	_, err := ReadSyncRecord(root, rel)
	require.ErrorContains(t, err, "full commit")
}

func TestGitCachePublishesVerifiedTreeAndReplacesCorruption(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	revision := "0123456789abcdef0123456789abcdef01234567"
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("original"), 0600))
	cached, err := StoreGitCache(root, "https://example.invalid/owner/repo.git", revision, source)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(cached, "SKILL.md"))
	second, err := StoreGitCache(root, "https://example.invalid/owner/repo.git", revision, source)
	require.NoError(t, err)
	require.Equal(t, cached, second, "valid immutable cache must be reused")
	require.NoError(t, os.WriteFile(filepath.Join(cached, "SKILL.md"), []byte("corrupt"), 0600))
	_, err = ReadGitCache(root, "https://example.invalid/owner/repo.git", revision)
	require.Error(t, err)
	rebuilt, err := StoreGitCache(root, "https://example.invalid/owner/repo.git", revision, source)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(rebuilt, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}

func TestRemoveLegacySyncIgnorePreservesUserRules(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("notes/\n/.skm-sync/\n/.skm-local/\n/forks/\n"), 0644))
	require.NoError(t, RemoveLegacySyncIgnore(root))
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, "notes/\n", string(data))
}
