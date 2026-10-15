package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func TestForkSwitchUpdateRollback(t *testing.T) {
	s, source, target := syncFixture(t)
	ctx := context.Background()
	_, err := s.Install(ctx, "review", InstallOptions{})
	require.NoError(t, err)
	upstream := filepath.Dir(currentSyncPath(t, s))
	res, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	require.True(t, res.Accepted)
	fork := filepath.Dir(currentSyncPath(t, s))
	require.NotEqual(t, upstream, fork)
	require.Equal(t, filepath.Join(s.Cfg.Root, "skills-fork", "local", "review"), fork)
	link, err := os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	require.Equal(t, fork, link)
	local := "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n"
	require.NoError(t, os.WriteFile(filepath.Join(fork, "guide.txt"), []byte(local), 0644))
	require.Contains(t, readSyncFile(t, filepath.Join(upstream, "guide.txt")), "one")
	require.Len(t, s.Scan(), 1, "fork must remain one logical Skill")
	writeFile(t, source, "guide.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n")
	res, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Contains(t, readSyncFile(t, currentSyncPath(t, s)), "ONE")
	require.Contains(t, readSyncFile(t, currentSyncPath(t, s)), "NINE")
	_, err = s.RollbackUpdate(ctx, "review", false)
	require.ErrorContains(t, err, "rollback_unavailable")
	require.Contains(t, readSyncFile(t, currentSyncPath(t, s)), "NINE")
	_, err = s.UseUpstream(ctx, "review", false)
	require.NoError(t, err)
	link, err = os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	require.Equal(t, upstream, link)
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.Contains(t, readSyncFile(t, filepath.Join(fork, "guide.txt")), "NINE")
	_, err = s.Fork(ctx, "review", engines.SyncOptions{})
	require.ErrorContains(t, err, "existing_fork")
	require.Len(t, s.Scan(), 1)
}
func TestForkFailureSceneAndAccept(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	writeFile(t, filepath.Dir(currentSyncPath(t, s)), "guide.txt", "local\n")
	writeFile(t, source, "guide.txt", "upstream\n")
	res, err := s.Update(ctx, "review", UpdateOptions{})
	require.Error(t, err)
	require.NotEmpty(t, res.AttemptPath)
	require.Contains(t, filepath.ToSlash(res.AttemptPath), ".skm/work/operations/")
	var attempt engines.SyncAttempt
	require.NoError(t, dal.ReadSyncJSON(filepath.Join(res.AttemptPath, "attempt.json"), &attempt))
	require.Equal(t, filepath.Base(res.AttemptPath), attempt.OperationID)
	require.Equal(t, "local\n", readSyncFile(t, currentSyncPath(t, s)))
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.ErrorContains(t, err, "pending_attempt")
	writeFile(t, filepath.Join(res.AttemptPath, "candidate"), "guide.txt", "resolved\n")
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.NoError(t, err)
	require.Equal(t, "resolved\n", readSyncFile(t, currentSyncPath(t, s)))
}

func TestForkCarriesCustomizationAcrossConsecutiveUpstreamUpdates(t *testing.T) {
	s, source, _ := syncFixture(t)
	ctx := context.Background()
	_, err := s.Fork(ctx, "review", engines.SyncOptions{})
	require.NoError(t, err)
	fork := filepath.Dir(currentSyncPath(t, s))
	writeFile(t, fork, "custom.txt", "local\n")
	// B -> U: upstream changes a distinct file, local addition remains.
	writeFile(t, source, "guide.txt", "U\n")
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "local\n", readSyncFile(t, filepath.Join(fork, "custom.txt")))
	require.Equal(t, "U\n", readSyncFile(t, filepath.Join(fork, "guide.txt")))
	// U -> V: upstream absorbs the same local addition. The merge must not
	// duplicate it or turn an already-absorbed customization into a conflict.
	writeFile(t, source, "guide.txt", "V\n")
	writeFile(t, source, "custom.txt", "local\n")
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, "local\n", readSyncFile(t, filepath.Join(fork, "custom.txt")))
	require.Equal(t, "V\n", readSyncFile(t, filepath.Join(fork, "guide.txt")))
	st, err := s.SyncStatus("review")
	require.NoError(t, err)
	require.Equal(t, "fork", st.Mode)
	require.NotEmpty(t, st.BaselineVersion)
}
