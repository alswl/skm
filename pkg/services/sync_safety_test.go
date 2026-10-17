package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/installer"
	"github.com/stretchr/testify/require"
)

func TestSyncPreviewStaleAndCancelledScene(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	before, err := dal.SyncTree(s.Cfg.Root)
	require.NoError(t, err)
	_, err = s.Update(ctx, "review", UpdateOptions{DryRun: true})
	require.Error(t, err)
	after, err := dal.SyncTree(s.Cfg.Root)
	require.NoError(t, err)
	require.Equal(t, before.Digest, after.Digest)
	res, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	writeFile(t, filepath.Join(res.AttemptPath, "candidate"), "guide.txt", "resolved\n")
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "later edit\n")
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "stale_input")
	require.Equal(t, "later edit\n", readSyncFile(t, currentSyncPath(t, s)))
	_, err = s.UseUpstream(ctx, "review", false)
	require.ErrorContains(t, err, "pending_attempt")
	_, err = s.CancelUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "stale_input")
	require.DirExists(t, res.AttemptPath)
}

func TestAcceptUpdateRejectsChangedRecordGeneration(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	res, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	writeFile(t, filepath.Join(res.AttemptPath, "candidate"), "guide.txt", "resolved\n")
	e := s.FindEntry("review")
	rec, rel, err := s.Repo.SyncRecord(e)
	require.NoError(t, err)
	rec.Generation++
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), "record.json"), rec))
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "stale_input")
	require.DirExists(t, res.AttemptPath)
}

func TestBaselineResolverUsesVerifiedGitCacheOffline(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	writeFile(t, source, "SKILL.md", frontmatter("review", "Review"))
	digest, err := dal.TreeFingerprint(source)
	require.NoError(t, err)
	revision := "0123456789abcdef0123456789abcdef01234567"
	repository := "https://example.invalid/owner/review.git"
	_, err = dal.StoreGitCache(root, repository, revision, source)
	require.NoError(t, err)
	s := newUpdateSvc(t, root)
	provider := "missing"
	rec := &common.SyncRecord{Origin: common.Origin{ProviderID: &provider, RepositoryAddress: repository}, UpstreamBase: common.BaselineRef{Kind: "git", Revision: revision, RawDigest: digest}}
	base, cleanup, err := s.resolveBaselineTree(context.Background(), rec, rec.UpstreamBase, filepath.Join(root, "missing-snapshot"))
	require.NoError(t, err)
	defer cleanup()
	got, err := dal.TreeFingerprint(base)
	require.NoError(t, err)
	require.Equal(t, digest, got)
}
func TestRollbackRefusesLaterEdits(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, source, "guide.txt", "updated\n")
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "later\n")
	_, err = s.RollbackUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "rollback_unavailable")
	require.Equal(t, "later\n", readSyncFile(t, currentSyncPath(t, s)))
}
func TestSyncArchiveProtectsInstallsAndMovesAssociation(t *testing.T) {
	s, source, target := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	_, err = s.Install(ctx, "review", InstallOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	_, err = s.ArchiveWithUninstall(ctx, "review", engines.LifecycleOptions{})
	require.ErrorContains(t, err, "pending_attempt")
	_, err = os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	_, err = s.CancelUpdate(ctx, "review", false)
	require.NoError(t, err)
	_, err = s.ArchiveWithUninstall(ctx, "review", engines.LifecycleOptions{})
	require.NoError(t, err)
	e, err := s.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, common.StatusArchived, e.Status)
	_, err = s.Unarchive(ctx, "review", engines.LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, "local\n", readSyncFile(t, currentSyncPath(t, s)))
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "fork", st.Mode)
}
func TestGitCloneKeepsForkAndBaseline(t *testing.T) {
	s, _, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "custom\n")
	run := func(args ...string) {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("-C", s.Cfg.Root, "init", "-q")
	run("-C", s.Cfg.Root, "add", ".")
	run("-C", s.Cfg.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "fixture")
	out, err := exec.Command("git", "-C", s.Cfg.Root, "ls-files").Output()
	require.NoError(t, err)
	require.NotContains(t, string(out), ".skm/work/")
	require.Contains(t, string(out), "skills-fork/")
	require.Contains(t, string(out), ".skm/")
	clone := filepath.Join(t.TempDir(), "clone")
	run("clone", "-q", s.Cfg.Root, clone)
	copySvc := newUpdateSvc(t, clone)
	st, err := copySvc.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "customized", st.Customization)
	require.Equal(t, "custom\n", readSyncFile(t, currentSyncPath(t, copySvc)))
}
func TestLegacyMissingBaselineAndExplicitMigration(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	writeFile(t, source, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, source, "guide.txt", "base\n")
	writeFile(t, root, "skills/local/review/SKILL.md", frontmatter("review", "Review"))
	writeFile(t, root, "skills/local/review/guide.txt", "custom\n")
	provider := "local"
	require.NoError(t, dal.WriteMeta(filepath.Join(root, "skills/local/review"), &common.Origin{Address: source, ProviderID: &provider}))
	s := newUpdateSvc(t, root)
	ctx := context.Background()
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.ErrorContains(t, err, "missing_baseline")
	require.NotContains(t, err.Error(), "format.json")
	require.Equal(t, "missing_baseline", result.ReasonCode)
	require.Contains(t, result.NextActions, "skm sync baseline review --help")
	raw, err := dal.SyncTree(source)
	require.NoError(t, err)
	opts := BaselineOptions{Source: source, Revision: "sha256:" + raw.Digest, NewStart: true}
	preview, err := s.SyncBaseline(ctx, "review", opts)
	require.NoError(t, err)
	opts.Accept = true
	opts.ExpectedTree = preview.Tree
	opts.ExpectedLocal = preview.Local
	_, err = s.SyncBaseline(ctx, "review", opts)
	require.NoError(t, err)
	require.Equal(t, "custom\n", readSyncFile(t, currentSyncPath(t, s)))
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "fork", st.Mode)
}
func TestReservedSourceMetadataFailsWithoutDiscarding(t *testing.T) {
	s, source, _ := syncFixture(t)
	writeFile(t, source, "meta.json", `{"user":"data"}`)
	_, err := s.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "reserved")
	require.FileExists(t, filepath.Join(source, "meta.json"))
}

func TestShareRequiresExplicitOriginalSourceForFork(t *testing.T) {
	s, _, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "custom\n")
	// Give the local fixture a shareable address without touching its content.
	e := s.FindEntry("review")
	rec, rel, err := s.Repo.SyncRecord(e)
	require.NoError(t, err)
	rec.Origin.Address = "https://github.com/example/review"
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(s.Cfg.Root, dal.SyncStateRel(rel), "record.json"), rec))
	_, err = s.CreateShare(ctx, []string{"review"}, nil)
	require.ErrorContains(t, err, "original-source")
	result, err := s.CreateOriginalSourceShare(ctx, []string{"review"}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Warnings)
}
func TestDeployDiscoversMaintainedForkOnce(t *testing.T) {
	s, _, _ := syncFixture(t)
	_, err := s.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entries, err := discoverDeploySkills(s.Cfg.Root, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, filepath.Dir(currentSyncPath(t, s)), entries[0].Path)
}

func TestRefreshFailureReportsAcceptedAndCanRetryInstall(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	scriptPath := filepath.Join(t.TempDir(), "target.sh")
	failureFlag := filepath.Join(t.TempDir(), "fail")
	script := `#!/bin/sh
IFS= read -r line
case "$line" in
 *'"action":"id"'*) echo '{"id":"copy","protocol_version":2}' ;;
 *'"action":"state"'*) echo '{"state":"installed"}' ;;
 *'"action":"install"'*) if [ -f '` + failureFlag + `' ]; then echo '{"error":{"code":"install_failed","message":"injected copy failure"}}'; else echo '{"result":true}'; fi ;;
 *) echo '{"result":true}' ;;
esac
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0755))
	plugin, err := NewTargetPlugin(scriptPath)
	require.NoError(t, err)
	target := common.InstallTarget{Name: "copy", Path: t.TempDir(), Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.PluginStrategy("copy")}}
	s.Installer = installer.NewInstaller([]common.InstallTarget{target}, map[string]installer.TargetDriver{"copy": externalTargetDriver{plugin}})
	_, err = s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(failureFlag, []byte("fail"), 0600))
	writeFile(t, source, "guide.txt", "updated\n")
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	require.Equal(t, common.ExitError, common.ExitCodeOf(err, 0))
	require.True(t, result.Accepted)
	require.Equal(t, "refresh_failed", result.ReasonCode)
	require.Len(t, result.Refresh, 1)
	require.False(t, result.Refresh[0].Success)
	require.Equal(t, "updated\n", readSyncFile(t, currentSyncPath(t, s)))
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	version := st.BaselineVersion
	require.NoError(t, os.Remove(failureFlag))
	_, err = s.Install(ctx, "review", InstallOptions{})
	require.NoError(t, err)
	st, err = s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, version, st.BaselineVersion)
}

func TestBaselineReassociationCannotOverwriteRetainedFork(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	fork := filepath.Dir(currentSyncPath(t, s))
	writeFile(t, fork, "guide.txt", "preserve\n")
	_, err = s.UseUpstream(ctx, "review", false)
	require.NoError(t, err)
	raw, err := dal.SyncTree(source)
	require.NoError(t, err)
	opts := BaselineOptions{Source: source, Revision: "sha256:" + raw.Digest, NewStart: true}
	result, err := s.SyncBaseline(ctx, "review", opts)
	require.NoError(t, err)
	opts.Accept = true
	opts.ExpectedTree = result.Tree
	opts.ExpectedLocal = result.Local
	_, err = s.SyncBaseline(ctx, "review", opts)
	require.ErrorContains(t, err, "existing_fork")
	require.Equal(t, "preserve\n", readSyncFile(t, filepath.Join(fork, "guide.txt")))
}

func TestInvalidUpdatePreservesSceneAndCurrentInstall(t *testing.T) {
	s, source, target := syncFixture(t)
	ctx := context.Background()
	_, err := s.Install(ctx, "review", InstallOptions{})
	require.NoError(t, err)
	writeFile(t, source, "SKILL.md", "---\nname: renamed\ndescription: incompatible identity\n---\nbody\n")
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	require.NotEmpty(t, result.AttemptPath)
	require.Contains(t, readSyncFile(t, filepath.Join(target, "review", "SKILL.md")), "name: review")
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "failed", st.AttemptState)
	require.NotEmpty(t, st.Failures)
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.Error(t, err)
}

func TestEditedCommunityMustBeForkedBeforeUpdate(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	_, err := s.Update(ctx, "review", UpdateOptions{})
	require.ErrorContains(t, err, "customization_requires_fork")
	require.Equal(t, "local\n", readSyncFile(t, currentSyncPath(t, s)))
	_, err = s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	require.NotEmpty(t, result.AttemptPath)
}
func TestImportedNormalizationIsNotMistakenForUserCustomization(t *testing.T) {
	source, root := t.TempDir(), t.TempDir()
	writeFile(t, source, "SKILL.md", "raw instructions\n")
	s := newUpdateSvc(t, root)
	imported, err := s.Import(context.Background(), source, ImportOptions{})
	require.NoError(t, err)
	result, err := s.Update(context.Background(), imported.Name, UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "current", result.Outcome)
	entry := s.FindEntry(imported.Name)
	rec, rel, err := s.Repo.SyncRecord(entry)
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(root, dal.SyncStateRel(rel), "upstream-baseline", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "raw instructions\n", string(raw))
	require.NotEmpty(t, rec.UpstreamVersion)
}

func TestUnresolvedMarkersAreRejectedButLiteralExamplesAreAllowed(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "unresolved patch markers")
	writeFile(t, filepath.Join(result.AttemptPath, "candidate"), "guide.txt", "Example:\n<<<<<<< ours\ntext\n>>>>>>> theirs\n")
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.NoError(t, err)
}

func TestMixedBatchContinuesAndReportsEachItem(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	good := t.TempDir()
	writeFile(t, good, "SKILL.md", frontmatter("other", "Other"))
	_, err = s.Import(ctx, good, ImportOptions{})
	require.NoError(t, err)
	writeFile(t, good, "new.txt", "new upstream\n")
	writeFile(t, s.Cfg.Root, "skills/local/legacy/SKILL.md", frontmatter("legacy", "Legacy"))
	id := "local"
	require.NoError(t, dal.WriteMeta(filepath.Join(s.Cfg.Root, "skills/local/legacy"), &common.Origin{Address: good, ProviderID: &id}))
	before, err := dal.SyncTree(s.Cfg.Root)
	require.NoError(t, err)
	preview := s.BatchUpdate(ctx, true)
	require.False(t, preview.Success)
	require.Contains(t, preview.Updated, "other")
	require.Len(t, preview.Failed, 2)
	after, err := dal.SyncTree(s.Cfg.Root)
	require.NoError(t, err)
	require.Equal(t, before.Digest, after.Digest)
	batch := s.BatchUpdate(ctx, false)
	require.False(t, batch.Success)
	require.Len(t, batch.Items, 3)
	require.Equal(t, 3, batch.Total)
	require.Contains(t, batch.Updated, "other")
	require.Len(t, batch.Failed, 2)
	require.Equal(t, "local\n", readSyncFile(t, currentSyncPath(t, s)))
	other := s.FindEntry("other")
	require.FileExists(t, filepath.Join(other.Path, "new.txt"))
	st, err := s.SyncStatus("other")
	require.NoError(t, err)
	raw, err := dal.SyncTree(good)
	require.NoError(t, err)
	require.Equal(t, "sha256:"+raw.Digest, st.BaselineVersion)
}

func TestDeletingCommunityKeepsRetainedForkAndWarns(t *testing.T) {
	s, _, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	fork := filepath.Dir(currentSyncPath(t, s))
	writeFile(t, fork, "guide.txt", "preserve\n")
	_, err = s.Delete(ctx, "review", engines.LifecycleOptions{Force: true})
	require.ErrorContains(t, err, "--upstream")
	_, err = s.UseUpstream(ctx, "review", false)
	require.NoError(t, err)
	result, err := s.Delete(ctx, "review", engines.LifecycleOptions{Force: true})
	require.NoError(t, err)
	require.NotEmpty(t, result.Warnings)
	require.Equal(t, "preserve\n", readSyncFile(t, filepath.Join(fork, "guide.txt")))
	require.Empty(t, s.Scan())
}

func TestTrackedCopyInstallsExactMaintainedResources(t *testing.T) {
	s, _, _ := syncFixture(t)
	_, err := s.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry := s.FindEntry("review")
	writeFile(t, entry.Path, "meta.json", `{"user_resource":true}`)
	target := common.InstallTarget{Name: "copy", Path: t.TempDir(), Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}}
	tx := &dal.FileTransaction{}
	_, err = s.Installer.CopySkill(tx, entry, target, false)
	require.NoError(t, err)
	tx.Commit()
	before, err := dal.SyncTree(entry.Path)
	require.NoError(t, err)
	installed, err := dal.SyncTree(filepath.Join(target.Path, "review"))
	require.NoError(t, err)
	require.Equal(t, before.Digest, installed.Digest)
}

func TestDeployRefusesInterruptedSourceRepository(t *testing.T) {
	s, _, _ := syncFixture(t)
	writeFile(t, s.Cfg.Root, ".skm/work/transactions/interrupted/journal.json", `{"Schema":2,"Entry":"skills/local/review","Moves":[{"Path":"skills/local/review","Before":"absent","After":"absent"}]}`)
	_, err := discoverDeploySkills(s.Cfg.Root, nil)
	require.ErrorContains(t, err, "recovery_required")
}

func TestOrdinarySyncWriteIsBlockedByInterruptedTransaction(t *testing.T) {
	s, source, _ := syncFixture(t)
	writeFile(t, s.Cfg.Root, ".skm/work/transactions/interrupted/journal.json", `{"Schema":2,"Entry":"skills/local/review","Moves":[{"Path":"skills/local/review","Before":"absent","After":"absent"}]}`)
	writeFile(t, source, "guide.txt", "upstream\n")
	_, err := s.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "recovery_required")
}

func TestShareIsBlockedByInterruptedTransaction(t *testing.T) {
	s, _, _ := syncFixture(t)
	writeFile(t, s.Cfg.Root, ".skm/work/transactions/interrupted/journal.json", `{"Schema":2,"Entry":"skills/local/review","Moves":[{"Path":"skills/local/review","Before":"absent","After":"absent"}]}`)
	_, err := s.CreateShare(context.Background(), []string{"review"}, nil)
	require.ErrorContains(t, err, "recovery_required")
}

func TestTrackedLifecycleIsBlockedByInterruptedTransaction(t *testing.T) {
	s, _, _ := syncFixture(t)
	writeFile(t, s.Cfg.Root, ".skm/work/transactions/interrupted/journal.json", `{"Schema":2,"Entry":"skills/local/review","Moves":[{"Path":"skills/local/review","Before":"absent","After":"absent"}]}`)
	_, err := s.Archive(context.Background(), "review", engines.LifecycleOptions{})
	require.ErrorContains(t, err, "recovery_required")
}

func TestSyncReasonCodeKeepsRecoveryAndMigrationActions(t *testing.T) {
	for _, code := range []string{"recovery_blocked", "migration_required", "migration_conflict"} {
		t.Run(code, func(t *testing.T) {
			require.Equal(t, code, syncReasonCode(fmt.Errorf("%s: detail", code)))
		})
	}
}

func TestRollbackLocatesPartiallyMissingEntryFromTransaction(t *testing.T) {
	s, _, _ := syncFixture(t)
	entry := s.FindEntry("review")
	require.NotNil(t, entry)
	rel := s.Repo.RelPath(entry.RepositoryPath)
	before, err := dal.TreeFingerprint(entry.RepositoryPath)
	require.NoError(t, err)
	dir := filepath.Join(s.Cfg.Root, ".skm", "work", "transactions", "interrupted")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, dal.CopySyncTree(entry.RepositoryPath, filepath.Join(dir, "old-0")))
	require.NoError(t, os.Rename(entry.RepositoryPath, filepath.Join(dir, "removed-0")))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(dir, "journal.json"), map[string]any{
		"Schema": 2, "Entry": rel, "BackupComplete": true, "State": "accepting",
		"Moves": []map[string]string{{"Path": rel, "Before": before, "After": "absent"}},
	}))

	result, err := s.RollbackUpdate(context.Background(), "review", false)
	require.NoError(t, err)
	require.Equal(t, "rolled_back", result.Outcome)
	require.Equal(t, rel, result.Entry)
	require.DirExists(t, entry.RepositoryPath)
	require.NoDirExists(t, dir)
}

func TestMigrateRollbackRestoresInterruptedMigration(t *testing.T) {
	root := t.TempDir()
	s := newUpdateSvc(t, root)
	path := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(path, 0700))
	writeFile(t, path, "SKILL.md", frontmatter("review", "Review"))
	before, err := dal.TreeFingerprint(path)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".skm"), 0700))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(root, ".skm", "format.json"), map[string]int{"schema_version": 2}))
	dir := filepath.Join(root, ".skm", "work", "transactions", "migration")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, dal.CopySyncTree(path, filepath.Join(dir, "old-0")))
	require.NoError(t, os.Rename(path, filepath.Join(dir, "removed-0")))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(dir, "journal.json"), map[string]any{
		"Schema": 2, "Entry": "migration", "BackupComplete": true, "State": "accepting",
		"Moves": []map[string]string{{"Path": "skills/local/review", "Before": before, "After": "absent"}},
	}))

	result, err := s.MigrateSync(context.Background(), false, "", true, false)
	require.NoError(t, err)
	require.Equal(t, "rolled_back", result.Outcome)
	require.True(t, result.Accepted)
	require.DirExists(t, path)
	require.NoDirExists(t, dir)
	require.NoDirExists(t, filepath.Join(root, ".skm"))
}

func TestMigrateRefreshesManagedLinkToMovedFork(t *testing.T) {
	s, _, target := legacySyncFixture(t, "active", false)
	oldFork := filepath.Join(s.Cfg.Root, "forks", dal.SyncKey("skills/local/review"))
	require.NoError(t, os.Symlink(oldFork, filepath.Join(target, "review")))
	preview, err := s.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	result, err := s.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Len(t, result.Refresh, 1)
	require.True(t, result.Refresh[0].Success)
	link, err := os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(s.Cfg.Root, dal.SyncForkRel("skills/local/review")), link)
}

func TestMigrateVerifiedGitBaselineUsesReconstructibleReference(t *testing.T) {
	s, _, _ := legacySyncFixture(t, "none", true)
	preview, err := s.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	result, err := s.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	require.True(t, result.Accepted)
	entry, err := s.ResolveEntry("review")
	require.NoError(t, err)
	rec, _, err := s.Repo.SyncRecord(entry)
	require.NoError(t, err)
	require.Equal(t, "git", rec.UpstreamBase.Kind)
	status, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "fetch_required", status.BaselineState)
}

func TestMigrateUpstreamEntryCanForkFromCentralBaseline(t *testing.T) {
	s, _, _ := legacySyncFixture(t, "none", false)
	preview, err := s.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	_, err = s.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.NoError(t, err)
	result, err := s.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	status, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "fork", status.Mode)
}

func TestMigrateRefusesLegacyPendingState(t *testing.T) {
	s, _, _ := legacySyncFixture(t, "retained", false)
	writeFile(t, s.Cfg.Root, ".skm-local/pending/interrupted/attempt.json", `{}`)
	preview, err := s.MigrateSync(context.Background(), false, "", false, false)
	require.NoError(t, err)
	_, err = s.MigrateSync(context.Background(), true, preview.ExpectedState, false, false)
	require.ErrorContains(t, err, "resolve legacy pending")
	require.DirExists(t, filepath.Join(s.Cfg.Root, ".skm-sync"))
	require.NoDirExists(t, filepath.Join(s.Cfg.Root, ".skm"))
}

func TestStructureConflictKeepsAttemptAndSupportsManualAcceptance(t *testing.T) {
	s, source, target := syncFixture(t)
	ctx := context.Background()
	// Establish a baseline with a directory before creating the customization.
	writeFile(t, source, "d/a", "base\n")
	_, err := s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	_, err = s.Install(ctx, "review", InstallOptions{})
	require.NoError(t, err)
	_, err = s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	fork := filepath.Dir(currentSyncPath(t, s))
	writeFile(t, fork, "d/b", "local add\n")
	before, err := dal.SyncTree(fork)
	require.NoError(t, err)
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	version := st.BaselineVersion
	require.NoError(t, os.RemoveAll(filepath.Join(source, "d")))
	writeFile(t, source, "d", "upstream replacement\n")
	result, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	require.Equal(t, "manual_required", result.Outcome)
	require.NotEmpty(t, result.AttemptPath)
	require.Contains(t, result.Failures, engines.SyncChange{Path: "d", Kind: "structure"})
	after, err := dal.SyncTree(fork)
	require.NoError(t, err)
	require.Equal(t, before.Digest, after.Digest)
	st, err = s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, version, st.BaselineVersion)
	require.Equal(t, "local add\n", readSyncFile(t, filepath.Join(target, "review", "d/b")))
	for _, file := range []string{"base/d/a", "local/d/b", "upstream/d", "candidate/d/b", "customization.patch", "attempt.json"} {
		require.FileExists(t, filepath.Join(result.AttemptPath, file))
	}
	attemptBefore, err := dal.SyncTree(result.AttemptPath)
	require.NoError(t, err)
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.ErrorContains(t, err, "pending_attempt")
	attemptAfter, err := dal.SyncTree(result.AttemptPath)
	require.NoError(t, err)
	require.Equal(t, attemptBefore.Digest, attemptAfter.Digest)
	candidate := filepath.Join(result.AttemptPath, "candidate")
	require.NoError(t, os.RemoveAll(filepath.Join(candidate, "d")))
	writeFile(t, candidate, "d", "manually resolved\n")
	accepted, err := s.AcceptUpdate(ctx, "review", false)
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	require.Equal(t, "manually resolved\n", readSyncFile(t, filepath.Join(target, "review", "d")))
	require.NoDirExists(t, result.AttemptPath)
}
