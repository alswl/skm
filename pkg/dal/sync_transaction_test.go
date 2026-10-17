package dal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncTreeFilesReportsWalkAndWorkerErrors(t *testing.T) {
	root := t.TempDir()
	require.Error(t, syncTreeFiles(filepath.Join(root, "missing")))
	for _, name := range []string{"00-broken", "99-broken"} {
		t.Run(name, func(t *testing.T) {
			tree := t.TempDir()
			for i := range 32 {
				require.NoError(t, os.WriteFile(filepath.Join(tree, fmt.Sprintf("file-%02d", i)), []byte("data"), 0600))
			}
			require.NoError(t, syncTreeFiles(tree))
			require.NoError(t, os.Symlink(filepath.Join(tree, "missing"), filepath.Join(tree, name)))
			require.Error(t, syncTreeFiles(tree), "a failed worker must prevent journal publication even after the walk finishes")
		})
	}
}

func TestSyncCommitDoesNotKeepSuccessfulRollback(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "data"), []byte("new"), 0600))
	path := filepath.Join(root, "skills", "foo")
	require.NoError(t, os.MkdirAll(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "data"), []byte("old"), 0600))
	old, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.NoError(t, CommitSync(root, "skills/foo", []SyncReplacement{{Path: "skills/foo", Source: src, Expected: old}}))
	_, err = RollbackReplacements(root, "skills/foo")
	require.ErrorContains(t, err, "rollback_unavailable")
	got, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.NotEqual(t, old, got)
}

func TestFinalSyncTransactionLeavesNoLegacyStoreOrTemp(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("new"), 0600))
	require.NoError(t, CommitFinalSync(root, "skills/local/review", []SyncReplacement{
		{Path: "skills/local/review", Source: src, Expected: "absent"},
	}))
	require.FileExists(t, filepath.Join(root, "skills", "local", "review", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, ".skm", "temp"))
	require.NoDirExists(t, filepath.Join(root, ".skm", "entries"))
	require.NoFileExists(t, filepath.Join(root, ".skm", "format.json"))
}

func TestFinalSyncInterruptedTransactionRequiresAndAllowsRollback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("old"), 0600))
	before, err := TreeFingerprint(path)
	require.NoError(t, err)
	dir := filepath.Join(root, ".skm", "temp", "transactions", "txn-interrupted")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, CopySyncTree(path, filepath.Join(dir, "old-0")))
	require.NoError(t, os.Rename(path, filepath.Join(dir, "removed-0")))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{
		Schema: 2, Entry: "skills/local/review", BackupComplete: true, State: "accepting",
		Moves: []syncMove{{Path: "skills/local/review", Before: before, After: "absent"}},
	}))
	require.ErrorContains(t, RecoverSync(root, true), "recovery_required")
	require.NoError(t, RollbackSync(root, "skills/local/review", false))
	require.FileExists(t, filepath.Join(path, "SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, ".skm", "temp"))
}

func TestCommitSyncReplacesAndAddsTreesWithModes(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "skills", "old")
	require.NoError(t, os.MkdirAll(old, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "data"), []byte("old"), 0600))
	oldDigest, err := TreeFingerprint(old)
	require.NoError(t, err)
	replacement, added := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(replacement, "script"), []byte("new"), 0751))
	require.NoError(t, os.Chmod(filepath.Join(replacement, "script"), 0751))
	require.NoError(t, os.WriteFile(filepath.Join(added, "data"), []byte("added"), 0640))

	require.NoError(t, CommitSync(root, "skills/old", []SyncReplacement{
		{Path: "skills/old", Source: replacement, Expected: oldDigest},
		{Path: "skills/new", Source: added, Expected: "absent"},
	}))
	info, err := os.Stat(filepath.Join(root, "skills", "old", "script"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0751), info.Mode().Perm())
	content, err := os.ReadFile(filepath.Join(root, "skills", "new", "data"))
	require.NoError(t, err)
	require.Equal(t, "added", string(content))
}
func TestSyncRecoveryRefusesExternalChanges(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	dir := filepath.Join(root, ".skm", "work", "transactions", "test")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(root, "content")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "x"), []byte("edited"), 0600))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "content", Moves: []syncMove{{Path: "content", Before: "before", After: "after"}}}))
	require.Error(t, RecoverSync(root, false))
	b, err := os.ReadFile(filepath.Join(path, "x"))
	require.NoError(t, err)
	require.Equal(t, "edited", string(b))
}

func TestRollbackSyncRestoresInterruptedFileReplacement(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	ignore := filepath.Join(root, ".gitignore")
	require.NoError(t, os.WriteFile(ignore, []byte("/.skm-sync/\n"), 0600))
	before, err := TreeFingerprint(ignore)
	require.NoError(t, err)
	dir := filepath.Join(root, ".skm", "work", "transactions", "migration")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, CopySyncTree(ignore, filepath.Join(dir, "old-0")))
	require.NoError(t, os.Rename(ignore, filepath.Join(dir, "removed-0")))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{
		Schema: 2, Entry: "migration", BackupComplete: true, State: "accepting",
		Moves: []syncMove{{Path: ".gitignore", Before: before, After: "absent"}},
	}))
	require.NoError(t, RollbackSync(root, "migration", false))
	data, err := os.ReadFile(ignore)
	require.NoError(t, err)
	require.Equal(t, "/.skm-sync/\n", string(data))
	require.NoDirExists(t, dir)
}

func TestRollbackSyncRestoresInterruptedTransactionAndProtectsEdits(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	dir := filepath.Join(root, ".skm", "work", "transactions", "interrupted")
	path := filepath.Join(root, "skills", "demo")
	old := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(old, "data"), []byte("old"), 0600))
	new := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(new, "data"), []byte("new"), 0600))
	before, err := TreeFingerprint(old)
	require.NoError(t, err)
	after, err := TreeFingerprint(new)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, CopySyncTree(old, filepath.Join(dir, "old-0")))
	require.NoError(t, CopySyncTree(new, path))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "skills/demo", BackupComplete: true, State: "accepting", Moves: []syncMove{{Path: "skills/demo", Before: before, After: after}}}))
	require.NoError(t, RollbackSync(root, "skills/demo", false))
	got, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.Equal(t, before, got)
	require.NoDirExists(t, dir)

	// A later editor change is not an acceptable intermediate state.
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, CopySyncTree(old, filepath.Join(dir, "old-0")))
	require.NoError(t, os.WriteFile(filepath.Join(path, "data"), []byte("edited"), 0600))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "skills/demo", BackupComplete: true, State: "accepting", Moves: []syncMove{{Path: "skills/demo", Before: before, After: after}}}))
	require.ErrorContains(t, RollbackSync(root, "skills/demo", false), "stale_input")
	require.DirExists(t, dir)
}

func TestSyncRecoverContinuesAndCleansUpRollbackStates(t *testing.T) {
	for _, state := range []string{"rolling_back", "rolled_back"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, InitSyncStore(root))
			dir := filepath.Join(root, ".skm", "work", "transactions", "rollback")
			old := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(old, "data"), []byte("old"), 0600))
			before, err := TreeFingerprint(old)
			require.NoError(t, err)
			path := filepath.Join(root, "skills", "demo")
			require.NoError(t, CopySyncTree(old, path))
			require.NoError(t, os.MkdirAll(dir, 0700))
			require.NoError(t, CopySyncTree(old, filepath.Join(dir, "old-0")))
			require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{
				Schema:         2,
				Entry:          "skills/demo",
				BackupComplete: true,
				State:          state,
				Moves:          []syncMove{{Path: "skills/demo", Before: before, After: "absent"}},
			}))

			require.NoError(t, RecoverSync(root, false))
			require.DirExists(t, path)
			require.NoDirExists(t, dir)
		})
	}
}

func TestRollbackSyncRejectsCorruptBackupAndPreservesModes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	dir := filepath.Join(root, ".skm", "work", "transactions", "interrupted")
	old := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(old, "script"), []byte("old"), 0751))
	require.NoError(t, os.Chmod(filepath.Join(old, "script"), 0751))
	new := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(new, "script"), []byte("new"), 0600))
	before, err := TreeFingerprint(old)
	require.NoError(t, err)
	after, err := TreeFingerprint(new)
	require.NoError(t, err)
	path := filepath.Join(root, "skills", "demo")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, CopySyncTree(old, filepath.Join(dir, "old-0")))
	require.NoError(t, CopySyncTree(new, path))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "skills/demo", BackupComplete: true, State: "accepting", Moves: []syncMove{{Path: "skills/demo", Before: before, After: after}}}))

	require.NoError(t, RollbackSync(root, "skills/demo", false))
	info, err := os.Stat(filepath.Join(path, "script"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0751), info.Mode().Perm())

	// A damaged backup must leave the current official tree and journal intact.
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "old-0"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "old-0", "script"), []byte("corrupt"), 0600))
	require.NoError(t, os.RemoveAll(path))
	require.NoError(t, CopySyncTree(new, path))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "skills/demo", BackupComplete: true, State: "accepting", Moves: []syncMove{{Path: "skills/demo", Before: before, After: after}}}))
	require.ErrorContains(t, RollbackSync(root, "skills/demo", false), "corrupt backup")
	got, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.Equal(t, after, got)
	require.DirExists(t, dir)
}

func TestSyncRecoveryRenameBoundaries(t *testing.T) {
	for _, phase := range []string{"prepared", "old-moved", "new-moved", "committed"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, InitSyncStore(root))
			dir := filepath.Join(root, ".skm", "work", "transactions", "interrupted")
			require.NoError(t, os.MkdirAll(dir, 0700))
			original := filepath.Join(root, "content")
			require.NoError(t, os.Mkdir(original, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(original, "data"), []byte("old"), 0600))
			staged := filepath.Join(dir, "new-0")
			require.NoError(t, os.Mkdir(staged, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(staged, "data"), []byte("new"), 0600))
			before, err := TreeFingerprint(original)
			require.NoError(t, err)
			after, err := TreeFingerprint(staged)
			require.NoError(t, err)
			journal := syncJournal{Schema: 2, Entry: "content", BackupComplete: true, State: "accepting", Moves: []syncMove{{Path: "content", Before: before, After: after}}, Committed: phase == "committed"}
			if journal.Committed {
				journal.State = "committed"
			}
			require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), journal))
			if phase != "prepared" {
				require.NoError(t, os.Rename(original, filepath.Join(dir, "old-0")))
			}
			if phase == "new-moved" || phase == "committed" {
				require.NoError(t, os.Rename(staged, original))
			}
			require.Error(t, RecoverSync(root, true))
			if phase != "committed" {
				require.ErrorContains(t, RecoverSync(root, false), "needs explicit rollback")
				return
			}
			require.NoError(t, RecoverSync(root, false))
			got, err := os.ReadFile(filepath.Join(original, "data"))
			require.NoError(t, err)
			want := "old"
			if phase == "committed" {
				want = "new"
			}
			require.Equal(t, want, string(got))
		})
	}
}

func TestSyncRecoveryAfterProcessExit(t *testing.T) {
	if root := os.Getenv("SKM_TEST_TXN_ROOT"); root != "" {
		phase := os.Getenv("SKM_TEST_TXN_PHASE")
		dir := filepath.Join(root, ".skm", "work", "transactions", "crash")
		before, _ := TreeFingerprint(filepath.Join(root, "content"))
		after, _ := TreeFingerprint(filepath.Join(dir, "new-0"))
		journal := syncJournal{Schema: 2, Entry: "content", BackupComplete: true, State: "accepting", Committed: phase == "committed", Moves: []syncMove{{Path: "content", Before: before, After: after}}}
		if journal.Committed {
			journal.State = "committed"
		}
		if err := WriteSyncJSON(filepath.Join(dir, "journal.json"), journal); err != nil {
			os.Exit(3)
		}
		if phase != "prepared" {
			if err := os.Rename(filepath.Join(root, "content"), filepath.Join(dir, "old-0")); err != nil {
				os.Exit(4)
			}
		}
		if phase == "new-moved" || phase == "committed" {
			if err := os.Rename(filepath.Join(dir, "new-0"), filepath.Join(root, "content")); err != nil {
				os.Exit(5)
			}
		}
		// Abrupt process death leaves the durable boundary, not Go rollback defers.
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		os.Exit(6)
	}
	for _, phase := range []string{"prepared", "old-moved", "new-moved", "committed"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, InitSyncStore(root))
			dir := filepath.Join(root, ".skm", "work", "transactions", "crash")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "new-0"), 0700))
			require.NoError(t, os.Mkdir(filepath.Join(root, "content"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "content", "data"), []byte("old"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "new-0", "data"), []byte("new"), 0600))
			cmd := exec.Command(os.Args[0], "-test.run=^TestSyncRecoveryAfterProcessExit$")
			cmd.Env = append(os.Environ(), "SKM_TEST_TXN_ROOT="+root, "SKM_TEST_TXN_PHASE="+phase)
			require.Error(t, cmd.Run())
			require.False(t, cmd.ProcessState.Success())
			if phase != "committed" {
				require.ErrorContains(t, RecoverSync(root, false), "needs explicit rollback")
				return
			}
			require.NoError(t, RecoverSync(root, false))
			got, err := os.ReadFile(filepath.Join(root, "content", "data"))
			require.NoError(t, err)
			want := "old"
			if phase == "committed" {
				want = "new"
			}
			require.Equal(t, want, string(got))
		})
	}
}

func TestSyncRecoveryRejectsReservedJournalAndEscapingRollback(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	gitdir := filepath.Join(root, ".git")
	require.NoError(t, os.Mkdir(gitdir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("protect"), 0600))
	hash, err := TreeFingerprint(gitdir)
	require.NoError(t, err)
	dir := filepath.Join(root, ".skm", "work", "transactions", "bad")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 2, Entry: "skills/local/demo", Moves: []syncMove{{Path: ".git", Before: "absent", After: hash}}}))
	require.Error(t, RecoverSync(root, false))
	require.FileExists(t, filepath.Join(gitdir, "HEAD"))
	require.NoError(t, os.RemoveAll(dir))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".skm", "work", "rollback")))
	_, err = RollbackReplacements(root, "skills/local/demo")
	require.Error(t, err)
}
