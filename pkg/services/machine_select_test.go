package services

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestSelectMachineDirectories(t *testing.T) {
	svc := &Services{Cfg: &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir()}}
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		dir := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", dir).CombinedOutput()
		require.NoError(t, err, string(output))
		require.NoError(t, svc.RegisterDirectories(ctx, map[string]string{id: dir}, []string{"shared"}))
	}
	require.NoError(t, svc.TagDirectories(ctx, []string{"a"}, []string{"extra"}, nil))
	selected, err := svc.SelectMachineDirectories([]string{"shared", "extra"})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	require.Equal(t, "a", selected[0].ID)
	require.Equal(t, "b", selected[1].ID)
	selected, err = svc.SelectMachineDirectories(nil)
	require.NoError(t, err)
	require.Empty(t, selected)
	_, err = svc.SelectMachineDirectories([]string{"INVALID"})
	require.Error(t, err)
}
