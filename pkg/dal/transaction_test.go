package dal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTransactionMoveStageBacksUpAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "entry")
	require.NoError(t, os.WriteFile(orig, []byte("original"), 0o644))
	staged := filepath.Join(dir, "staged")
	require.NoError(t, os.WriteFile(staged, []byte("new"), 0o644))

	tx := &FileTransaction{}
	require.NoError(t, tx.MoveStage(staged, orig))
	got, _ := os.ReadFile(orig)
	require.Equal(t, "new", string(got))

	require.NoError(t, tx.Rollback())
	got, _ = os.ReadFile(orig)
	require.Equal(t, "original", string(got), "rollback must restore the original entry")
}

func TestTransactionMoveNoBackupRemovesOnRollback(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged")
	dst := filepath.Join(dir, "new-entry")
	require.NoError(t, os.WriteFile(staged, []byte("x"), 0o644))

	tx := &FileTransaction{}
	require.NoError(t, tx.MoveStage(staged, dst))
	require.NoError(t, tx.Rollback())
	_, err := os.Stat(dst)
	require.True(t, os.IsNotExist(err), "rollback should remove the moved dst")
}

func TestTransactionCreateLinkRollback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "src")
	link := filepath.Join(dir, "lnk")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	tx := &FileTransaction{}
	require.NoError(t, tx.CreateLink(target, link))
	got, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, target, got)

	require.NoError(t, tx.Rollback())
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err), "rollback should remove created link")
}

func TestTransactionRefusesLinkOverExisting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "conflict")
	require.NoError(t, os.WriteFile(dst, []byte("user file"), 0o644))

	tx := &FileTransaction{}
	err := tx.CreateLink(filepath.Join(dir, "src"), dst)
	require.Error(t, err, "must refuse to link over a user file")
	got, _ := os.ReadFile(dst)
	require.Equal(t, "user file", string(got), "user file must be untouched")
}

func TestLockExclusiveAndRelease(t *testing.T) {
	dir := t.TempDir()
	l1, err := AcquireLockWait(context.Background(), dir, 200*time.Millisecond)
	require.NoError(t, err)
	defer l1.Release()

	_, err = AcquireLockWait(context.Background(), dir, 200*time.Millisecond)
	require.Error(t, err, "second acquire must fail while lock is held")

	l1.Release()
	l2, err := AcquireLockWait(context.Background(), dir, 200*time.Millisecond)
	require.NoError(t, err, "lock should be acquirable after release")
	l2.Release()
}

func TestCommitRemovesBackups(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "entry")
	require.NoError(t, os.WriteFile(orig, []byte("original"), 0o644))
	staged := filepath.Join(dir, "staged")
	require.NoError(t, os.WriteFile(staged, []byte("new"), 0o644))

	tx := &FileTransaction{}
	require.NoError(t, tx.MoveStage(staged, orig))
	tx.Commit()
	// The backup of the replaced target must be gone after commit.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.skm-bak-*"))
	require.Empty(t, matches, "commit must not leave .skm-bak-* residue")
	got, _ := os.ReadFile(orig)
	require.Equal(t, "new", string(got))
}

func TestLockNameStablePerRoot(t *testing.T) {
	a := LockName("/a/b/repo")
	b := LockName("/a/b/repo")
	require.Equal(t, a, b)
	require.NotEqual(t, a, LockName("/a/b/other"))
}
