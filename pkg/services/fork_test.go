package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	link, err := os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	require.Equal(t, fork, link)
	local := "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n"
	require.NoError(t, os.WriteFile(filepath.Join(fork, "guide.txt"), []byte(local), 0644))
	writeFile(t, source, "guide.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n")
	res, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Contains(t, readSyncFile(t, currentSyncPath(t, s)), "ONE")
	require.Contains(t, readSyncFile(t, currentSyncPath(t, s)), "NINE")
	_, err = s.RollbackUpdate(ctx, "review", false)
	require.NoError(t, err)
	require.Equal(t, local, readSyncFile(t, currentSyncPath(t, s)))
	_, err = s.UseUpstream(ctx, "review", false)
	require.NoError(t, err)
	link, err = os.Readlink(filepath.Join(target, "review"))
	require.NoError(t, err)
	require.Equal(t, upstream, link)
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.NoError(t, err)
	require.Equal(t, local, readSyncFile(t, filepath.Join(fork, "guide.txt")))
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
	require.Equal(t, "local\n", readSyncFile(t, currentSyncPath(t, s)))
	_, err = s.Update(ctx, "review", UpdateOptions{})
	require.ErrorContains(t, err, "pending_attempt")
	writeFile(t, filepath.Join(res.AttemptPath, "candidate"), "guide.txt", "resolved\n")
	_, err = s.AcceptUpdate(ctx, "review", false)
	require.NoError(t, err)
	require.Equal(t, "resolved\n", readSyncFile(t, currentSyncPath(t, s)))
}
