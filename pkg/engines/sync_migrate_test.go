package engines

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestMigrateLegacyMovesForkAndRecord(t *testing.T) {
	root := t.TempDir()
	rel := "skills/local/demo"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("notes/\n/.skm-sync/\n/.skm-local/\n/forks/\n"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, rel), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0600))
	legacy := filepath.Join(root, ".skm-sync")
	require.NoError(t, os.MkdirAll(filepath.Join(legacy, "entries", dal.SyncKey(rel), "upstream-baseline"), 0700))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(legacy, "format.json"), map[string]int{"schema_version": 1}))
	base := filepath.Join(legacy, "entries", dal.SyncKey(rel), "upstream-baseline")
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\n"), 0600))
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "base")
	revision := git("rev-parse", "HEAD")
	require.NoError(t, dal.CopySyncTree(source, base))
	digest, err := dal.TreeFingerprint(base)
	require.NoError(t, err)
	oldFork := filepath.Join("forks", dal.SyncKey(rel))
	require.NoError(t, os.MkdirAll(filepath.Join(root, oldFork), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, oldFork, "SKILL.md"), []byte("---\nname: demo\ndescription: demo\n---\ncustom"), 0600))
	provider := "git"
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: "demo", Generation: 1, Origin: common.Origin{Address: source, RepositoryAddress: source, ProviderID: &provider, Revision: revision}, Active: "fork", ForkPath: oldFork, UpstreamVersion: revision, ForkVersion: revision, UpstreamDigest: digest, ForkDigest: digest}
	_, verified := verifiedLegacyGitBaseline(t.TempDir(), rec.Origin, revision, digest)
	require.True(t, verified)
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
	require.Equal(t, "git", got.UpstreamBase.Kind)
	require.Equal(t, revision, got.UpstreamBase.Revision)
	require.NoDirExists(t, filepath.Join(root, ".skm", "baselines", digest))
	require.NoDirExists(t, filepath.Join(root, dal.SyncStateRel(rel), "upstream-baseline"))
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	require.Contains(t, string(ignore), "notes/\n")
	require.NotContains(t, string(ignore), ".skm-sync/")
	require.NotContains(t, string(ignore), ".skm-local/")
	require.NotContains(t, string(ignore), "/forks/")
	require.Contains(t, string(ignore), "/.skm/cache/")
	require.Contains(t, string(ignore), "/.skm/work/")
	status, err := repo.SyncStatus(&common.Entry{Name: "demo", Kind: common.KindSkill, Path: filepath.Join(root, got.ForkPath), RepositoryPath: filepath.Join(root, rel)})
	require.NoError(t, err)
	require.Equal(t, "fetch_required", status.BaselineState)
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
