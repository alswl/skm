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

func TestSyncCommitAndRollback(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "data"), []byte("new"), 0600))
	path := filepath.Join(root, "skills", "foo")
	require.NoError(t, os.MkdirAll(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "data"), []byte("old"), 0600))
	old, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.NoError(t, CommitSync(root, "skills/foo", []SyncReplacement{{Path: "skills/foo", Source: src, Expected: old}}, true))
	moves, err := RollbackReplacements(root, "skills/foo")
	require.NoError(t, err)
	require.NoError(t, CommitSync(root, "skills/foo", moves, false))
	got, err := TreeFingerprint(path)
	require.NoError(t, err)
	require.Equal(t, old, got)
}
func TestSyncRecoveryRefusesExternalChanges(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, InitSyncStore(root))
	dir := filepath.Join(root, ".skm-local", "transactions", "test")
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(root, "content")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "x"), []byte("edited"), 0600))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 1, Entry: "content", Moves: []syncMove{{Path: "content", Before: "before", After: "after"}}}))
	require.Error(t, RecoverSync(root, false))
	b, err := os.ReadFile(filepath.Join(path, "x"))
	require.NoError(t, err)
	require.Equal(t, "edited", string(b))
}

func TestSyncRecoveryRenameBoundaries(t *testing.T) {
	for _, phase := range []string{"prepared", "old-moved", "new-moved", "committed"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, InitSyncStore(root))
			dir := filepath.Join(root, ".skm-local", "transactions", "interrupted")
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
			journal := syncJournal{Schema: 1, Entry: "content", KeepRollback: true, Moves: []syncMove{{Path: "content", Before: before, After: after}}, Committed: phase == "committed"}
			require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), journal))
			if phase != "prepared" {
				require.NoError(t, os.Rename(original, filepath.Join(dir, "old-0")))
			}
			if phase == "new-moved" || phase == "committed" {
				require.NoError(t, os.Rename(staged, original))
			}
			require.Error(t, RecoverSync(root, true))
			require.NoError(t, RecoverSync(root, false))
			require.NoError(t, RecoverSync(root, false))
			got, err := os.ReadFile(filepath.Join(original, "data"))
			require.NoError(t, err)
			want := "old"
			if phase == "committed" {
				want = "new"
				_, err = RollbackReplacements(root, "content")
				require.NoError(t, err)
			}
			require.Equal(t, want, string(got))
		})
	}
}

func TestSyncRecoveryAfterProcessExit(t *testing.T) {
	if root := os.Getenv("SKM_TEST_TXN_ROOT"); root != "" {
		phase := os.Getenv("SKM_TEST_TXN_PHASE")
		dir := filepath.Join(root, ".skm-local", "transactions", "crash")
		before, _ := TreeFingerprint(filepath.Join(root, "content"))
		after, _ := TreeFingerprint(filepath.Join(dir, "new-0"))
		if err := WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 1, Entry: "content", KeepRollback: true, Committed: phase == "committed", Moves: []syncMove{{Path: "content", Before: before, After: after}}}); err != nil {
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
			dir := filepath.Join(root, ".skm-local", "transactions", "crash")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "new-0"), 0700))
			require.NoError(t, os.Mkdir(filepath.Join(root, "content"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "content", "data"), []byte("old"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "new-0", "data"), []byte("new"), 0600))
			cmd := exec.Command(os.Args[0], "-test.run=^TestSyncRecoveryAfterProcessExit$")
			cmd.Env = append(os.Environ(), "SKM_TEST_TXN_ROOT="+root, "SKM_TEST_TXN_PHASE="+phase)
			require.Error(t, cmd.Run())
			require.False(t, cmd.ProcessState.Success())
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
	dir := filepath.Join(root, ".skm-local", "transactions", "bad")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, WriteSyncJSON(filepath.Join(dir, "journal.json"), syncJournal{Schema: 1, Entry: "skills/local/demo", Moves: []syncMove{{Path: ".git", Before: "absent", After: hash}}}))
	require.Error(t, RecoverSync(root, false))
	require.FileExists(t, filepath.Join(gitdir, "HEAD"))
	require.NoError(t, os.RemoveAll(dir))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".skm-local", "rollback")))
	_, err = RollbackReplacements(root, "skills/local/demo")
	require.Error(t, err)
}
