package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func TestMigrateV2ActiveForkToColocatedMeta(t *testing.T) {
	svc, _, _ := syncFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	writeFile(t, entry.Path, "personal.txt", "mine\n")
	preview, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	require.NotEmpty(t, preview.ExpectedState)
	result, err := svc.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	entry, err = svc.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, "fork", entry.SyncView.Mode)
	require.FileExists(t, filepath.Join(entry.Path, "personal.txt"))
	_, err = dal.ReadForkMeta(entry.Path)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "entries"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "baselines"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "cache"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "work"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "temp"))
	require.NoFileExists(t, filepath.Join(svc.Cfg.Root, ".skm", "format.json"))
	ignore, err := os.ReadFile(filepath.Join(svc.Cfg.Root, ".gitignore"))
	require.NoError(t, err)
	require.Contains(t, string(ignore), "/.skm/temp/")
	again, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	require.Equal(t, "current", again.Outcome)
	require.Empty(t, again.Entries)
}

func TestMigrateV2RetainedGitForkKeepsItsBase(t *testing.T) {
	git := newGitSyncSource(t)
	root := t.TempDir()
	require.NoError(t, dal.InitSyncStore(root))
	stage := t.TempDir()
	for _, name := range []string{"SKILL.md", "guide.txt"} {
		out, err := exec.Command("git", "-C", git.Path, "show", git.Previous+":"+name).Output()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(stage, name), out, 0600))
	}
	provider := "github"
	origin := &common.Origin{Address: "https://github.com/example/review", ProviderID: &provider, RepositoryAddress: git.Path, Revision: git.Previous}
	_, err := engines.NewRepository(root).ImportStaged(context.Background(), stage, provider, "", false, origin)
	require.NoError(t, err)
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	_, err = svc.UseUpstream(context.Background(), "review", false)
	require.NoError(t, err)
	preview, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	_, err = svc.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, "upstream", entry.SyncView.Mode)
	require.Equal(t, "retained", entry.SyncView.ForkState)
	meta, err := dal.ReadSkillMeta(entry.Path)
	require.NoError(t, err)
	require.Equal(t, git.Previous, meta.Revision)
	forkMeta, err := dal.ReadForkMeta(entry.AlternatePath)
	require.NoError(t, err)
	require.Equal(t, git.Previous, forkMeta.BaseRevision)
}

func TestMigrateMetaOnlySnapshotWithoutOldStore(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", "local", "review")
	writeFile(t, path, "SKILL.md", frontmatter("review", "Review"))
	provider := "local"
	require.NoError(t, dal.WriteMeta(path, &common.Origin{Address: t.TempDir(), ProviderID: &provider, Path: "skills/local/review"}))
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	preview, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	require.Equal(t, []string{"review"}, preview.Entries)
	_, err = svc.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	meta, err := dal.ReadSkillMeta(path)
	require.NoError(t, err)
	require.Equal(t, 3, meta.SchemaVersion)
	require.Equal(t, "snapshot", meta.RevisionKind)
	require.NoDirExists(t, filepath.Join(root, ".skm", "temp"))
}

func TestMigrateMetaOnlyGitFindsExactHistoricalRevision(t *testing.T) {
	git := newGitSyncSource(t)
	root := t.TempDir()
	path := filepath.Join(root, "skills", "github", "review")
	require.NoError(t, os.MkdirAll(path, 0700))
	for _, name := range []string{"SKILL.md", "guide.txt"} {
		out, err := exec.Command("git", "-C", git.Path, "show", git.Previous+":"+name).Output()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(path, name), out, 0600))
	}
	provider := "github"
	require.NoError(t, dal.WriteMeta(path, &common.Origin{Address: "https://github.com/example/review", ProviderID: &provider, RepositoryAddress: git.Path, Path: "skills/github/review"}))
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	preview, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	_, err = svc.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	meta, err := dal.ReadSkillMeta(path)
	require.NoError(t, err)
	require.Equal(t, "git", meta.RevisionKind)
	require.Equal(t, git.Previous, meta.Revision)
	require.NoDirExists(t, filepath.Join(root, ".skm", "temp"))
}

func TestMigrateLegacyUnknownGitRevisionPreservesOldStore(t *testing.T) {
	svc, _, _ := legacySyncFixture(t, "none", true)
	rel := "skills/local/review"
	path := filepath.Join(svc.Cfg.Root, ".skm-sync", "entries", dal.SyncKey(rel), "record.json")
	var rec common.SyncRecord
	require.NoError(t, dal.ReadSyncJSON(path, &rec))
	rec.UpstreamVersion = "unknown"
	require.NoError(t, dal.WriteSyncJSON(path, rec))
	_, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.ErrorContains(t, err, "unprovable Git revision")
	require.DirExists(t, filepath.Join(svc.Cfg.Root, ".skm-sync"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm"))
	require.FileExists(t, filepath.Join(svc.Cfg.Root, rel, "SKILL.md"))
}

func TestMigrateV2RefusesPendingAttemptWithoutRetiringState(t *testing.T) {
	svc, _, _ := syncFixture(t)
	writeFile(t, svc.Cfg.Root, ".skm/work/operations/pending/attempt.json", `{}`)
	_, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.ErrorContains(t, err, "pending old operation")
	require.DirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "entries"))
	require.FileExists(t, filepath.Join(svc.Cfg.Root, "skills", "local", "review", "SKILL.md"))
}

func TestMigrateEmptyV2StoreRetiresItsFormat(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, dal.InitSyncStore(root))
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	preview, err := svc.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	require.Equal(t, "planned", preview.Outcome)
	_, err = svc.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(root, ".skm", "format.json"))
	require.NoDirExists(t, filepath.Join(root, ".skm", "entries"))
}
