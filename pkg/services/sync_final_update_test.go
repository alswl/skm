package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func finalGitFixture(t *testing.T) (*Services, gitSyncSource) {
	t.Helper()
	git := newGitSyncSource(t)
	root := t.TempDir()
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
	return svc, git
}

func TestFinalGitUpdateUsesExactOldCommitAndLeavesNoCache(t *testing.T) {
	svc, git := finalGitFixture(t)
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "updated", result.Outcome)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(entry.Path, "guide.txt"))
	require.NoError(t, err)
	require.Equal(t, "upstream\n", string(data))
	meta, err := dal.ReadSkillMeta(entry.Path)
	require.NoError(t, err)
	require.Equal(t, git.Head, meta.Revision)
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "temp"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "cache"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "entries"))
}

func TestFinalGitUpdatePreservesActiveForkCustomization(t *testing.T) {
	svc, git := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(entry.Path, "personal.txt"), []byte("mine\n"), 0600))
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "updated", result.Outcome)
	entry, err = svc.ResolveEntry("review")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(entry.Path, "personal.txt"))
	data, err := os.ReadFile(filepath.Join(entry.Path, "guide.txt"))
	require.NoError(t, err)
	require.Equal(t, "upstream\n", string(data))
	forkMeta, err := dal.ReadForkMeta(entry.Path)
	require.NoError(t, err)
	require.Equal(t, git.Head, forkMeta.BaseRevision)
	status, err := svc.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "active", status.ForkState)
	require.True(t, strings.HasPrefix(status.BaselineVersion, git.Head))
}

func TestFinalGitDiffUsesForkBaseAndIgnoresManagementMeta(t *testing.T) {
	svc, _ := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	writeFile(t, entry.Path, "personal.txt", "mine\n")
	diff, err := svc.SyncDiff(context.Background(), "review")
	require.NoError(t, err)
	require.Contains(t, diff, "added personal.txt")
	require.NotContains(t, diff, "meta.json")
	_, err = svc.UseUpstream(context.Background(), "review", false)
	require.NoError(t, err)
	diff, err = svc.SyncDiff(context.Background(), "review")
	require.NoError(t, err)
	require.Contains(t, diff, "added personal.txt", "inactive fork keeps its customization")
}

func TestFinalInactiveForkSurvivesUpstreamUpdateAndCanBeReselected(t *testing.T) {
	svc, git := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	writeFile(t, entry.Path, "personal.txt", "mine\n")
	forkPath := entry.Path
	before, err := dal.TreeFingerprint(forkPath)
	require.NoError(t, err)
	_, err = svc.UseUpstream(context.Background(), "review", false)
	require.NoError(t, err)
	_, err = svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	after, err := dal.TreeFingerprint(forkPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err = svc.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, forkPath, entry.Path)
	forkMeta, err := dal.ReadForkMeta(forkPath)
	require.NoError(t, err)
	require.Equal(t, git.Previous, forkMeta.BaseRevision)
	_, err = svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	forkMeta, err = dal.ReadForkMeta(forkPath)
	require.NoError(t, err)
	require.Equal(t, git.Head, forkMeta.BaseRevision)
	require.FileExists(t, filepath.Join(forkPath, "personal.txt"))
}

func TestFinalGitRepeatedUpdateDoesNotDuplicateUpstreamedCustomization(t *testing.T) {
	svc, git := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	writeFile(t, entry.Path, "personal.txt", "mine\n")
	_, err = svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	writeFile(t, git.Path, "personal.txt", "mine\n")
	cmd := exec.Command("git", "-C", git.Path, "add", "personal.txt")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	cmd = exec.Command("git", "-C", git.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "upstreamed customization")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "updated", result.Outcome)
	entry, err = svc.ResolveEntry("review")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(entry.Path, "personal.txt"))
	require.NoError(t, err)
	require.Equal(t, "mine\n", string(data))
	third, err := exec.Command("git", "-C", git.Path, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	forkMeta, err := dal.ReadForkMeta(entry.Path)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(third)), forkMeta.BaseRevision)
}

func TestFinalSnapshotCannotUpdateAndBatchSkipsIt(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	writeFile(t, stage, "SKILL.md", frontmatter("review", "Review"))
	provider := "local"
	origin := &common.Origin{Address: stage, ProviderID: &provider}
	_, err := engines.NewRepository(root).ImportStaged(context.Background(), stage, provider, "", false, origin)
	require.NoError(t, err)
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	require.False(t, svc.Updatable(entry))
	_, err = svc.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "not_applicable")
	batch := svc.BatchUpdate(context.Background(), false)
	require.Equal(t, []string{"review"}, batch.Skipped)
	require.Empty(t, batch.Failed)
	writeFile(t, stage, "guide.txt", "changed")
	_, err = engines.NewRepository(root).ImportStaged(context.Background(), stage, provider, "", true, origin)
	require.ErrorContains(t, err, "existing skill")
}

func TestFinalGitUpdateMissingExactHistoryPreservesFormalContent(t *testing.T) {
	svc, git := finalGitFixture(t)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	before, err := dal.TreeFingerprint(entry.Path)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(git.Path))
	_, err = svc.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "exact Git base")
	after, err := dal.TreeFingerprint(entry.Path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestFinalGitConflictCanBeCancelledWithoutChangingFormalContent(t *testing.T) {
	svc, _ := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(entry.Path, "guide.txt"), []byte("local\n"), 0600))
	before, err := dal.TreeFingerprint(entry.Path)
	require.NoError(t, err)
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "patch_failed")
	require.Equal(t, "manual_required", result.Outcome)
	require.DirExists(t, result.AttemptPath)
	status, err := svc.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "failed", status.AttemptState)
	_, err = svc.CancelUpdate(context.Background(), "review", false)
	require.NoError(t, err)
	check, err := dal.TreeFingerprint(entry.Path)
	require.NoError(t, err)
	require.Equal(t, before, check)
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "temp"))
}

func TestFinalGitConflictAcceptsResolvedCandidate(t *testing.T) {
	svc, git := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(entry.Path, "guide.txt"), []byte("local\n"), 0600))
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "patch_failed")
	require.DirExists(t, result.AttemptPath)
	require.NoError(t, os.WriteFile(filepath.Join(result.AttemptPath, "candidate", "guide.txt"), []byte("resolved\n"), 0600))
	accepted, err := svc.AcceptUpdate(context.Background(), "review", false)
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	entry, err = svc.ResolveEntry("review")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(entry.Path, "guide.txt"))
	require.NoError(t, err)
	require.Equal(t, "resolved\n", string(data))
	forkMeta, err := dal.ReadForkMeta(entry.Path)
	require.NoError(t, err)
	require.Equal(t, git.Head, forkMeta.BaseRevision)
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "temp"))
}

func TestFinalGitConflictAcceptRefusesExternalFormalEdit(t *testing.T) {
	svc, _ := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	writeFile(t, entry.Path, "guide.txt", "local\n")
	result, err := svc.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "patch_failed")
	writeFile(t, entry.Path, "guide.txt", "external edit\n")
	_, err = svc.AcceptUpdate(context.Background(), "review", false)
	require.ErrorContains(t, err, "stale_input")
	require.DirExists(t, result.AttemptPath)
	data, err := os.ReadFile(filepath.Join(entry.Path, "guide.txt"))
	require.NoError(t, err)
	require.Equal(t, "external edit\n", string(data))
}

func TestFinalInterruptedReplacementIsNotServedAndCanRollBack(t *testing.T) {
	svc, _ := finalGitFixture(t)
	entry, err := svc.ResolveEntry("review")
	require.NoError(t, err)
	upstream := entry.RepositoryPath
	rel, err := filepath.Rel(svc.Cfg.Root, upstream)
	require.NoError(t, err)
	before, err := dal.TreeFingerprint(upstream)
	require.NoError(t, err)
	dir := filepath.Join(svc.Cfg.Root, ".skm", "temp", "transactions", "txn-interrupted")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, dal.CopySyncTree(upstream, filepath.Join(dir, "old-0")))
	require.NoError(t, os.Rename(upstream, filepath.Join(dir, "removed-0")))
	require.NoError(t, dal.WriteSyncJSON(filepath.Join(dir, "journal.json"), map[string]any{
		"Schema": 2, "Entry": rel, "BackupComplete": true, "State": "accepting",
		"Moves": []map[string]string{{"Path": rel, "Before": before, "After": "absent"}},
	}))
	for _, candidate := range svc.Scan() {
		require.NotEqual(t, common.StatusActive, candidate.Status)
	}
	result, err := svc.RollbackUpdate(context.Background(), "review", false)
	require.NoError(t, err)
	require.Equal(t, "rolled_back", result.Outcome)
	require.FileExists(t, filepath.Join(upstream, "SKILL.md"))
	require.NoDirExists(t, filepath.Join(svc.Cfg.Root, ".skm", "temp"))
}

func TestFinalOfflineCloneCanListAndInstallWithoutHistory(t *testing.T) {
	svc, git := finalGitFixture(t)
	_, err := svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.NoError(t, err)
	clone := t.TempDir()
	for _, name := range []string{"skills", "skills-fork"} {
		require.NoError(t, dal.CopySyncTree(filepath.Join(svc.Cfg.Root, name), filepath.Join(clone, name)))
	}
	require.NoError(t, os.RemoveAll(git.Path))
	target := skillTarget("offline", t.TempDir())
	offline, err := New(newCfg(clone, []common.InstallTarget{target}), common.NewLogger(false))
	require.NoError(t, err)
	entry, err := offline.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, "fork", entry.SyncView.Mode)
	status, err := offline.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "unknown", status.Customization)
	_, err = offline.Install(context.Background(), "review", InstallOptions{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(target.Path, "review", "SKILL.md"))
	_, err = offline.Update(context.Background(), "review", UpdateOptions{})
	require.ErrorContains(t, err, "exact Git base")
}
