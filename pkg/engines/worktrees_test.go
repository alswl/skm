package engines

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverWorktrees(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	main := filepath.Join(parent, "main")
	linked := filepath.Join(parent, "linked")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	run("init", "-q", main)
	run("-C", main, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "initial")
	run("-C", main, "worktree", "add", "--detach", "-q", linked)
	require.NoError(t, os.Symlink(main, filepath.Join(parent, "alias")))
	invalid := filepath.Join(parent, "invalid")
	require.NoError(t, os.Mkdir(invalid, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(invalid, ".git"), []byte("invalid"), 0o644))
	bare := filepath.Join(parent, "bare.git")
	run("init", "--bare", "-q", bare)
	found, err := DiscoverWorktrees(context.Background(), parent)
	require.NoError(t, err)
	require.Equal(t, []string{linked, main}, found)
	report, err := DiscoverWorktreesReport(context.Background(), parent)
	require.NoError(t, err)
	require.Len(t, report, 3)
	require.Equal(t, invalid, report[0].Path)
	require.Contains(t, report[0].Error, "cannot inspect Git root")
	require.Empty(t, report[1].Error)
	require.Empty(t, report[2].Error)
}

func TestDiscoverWorktreesRespectsBoundariesAndCancellation(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "outside")
	output, err := exec.Command("git", "init", "-q", outside).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, os.Symlink(outside, filepath.Join(parent, "outside-alias")))
	items, err := DiscoverWorktreesReport(context.Background(), parent)
	require.NoError(t, err)
	require.Empty(t, items)
	file := filepath.Join(parent, "not-directory")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err = DiscoverWorktreesReport(context.Background(), file)
	require.ErrorContains(t, err, "not a directory")
	project := filepath.Join(parent, "project")
	output, err = exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DiscoverWorktreesReport(ctx, parent)
	require.Error(t, err)
}

func TestMachineGitSlotStatus(t *testing.T) {
	root := t.TempDir()
	output, err := exec.Command("git", "init", "-q", root).CombinedOutput()
	require.NoError(t, err, string(output))
	slot := filepath.Join(root, ".agents", "skills", "review")
	status, err := MachineGitSlotStatus(root, slot)
	require.NoError(t, err)
	require.Equal(t, "unignored", status)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/.agents/skills/\n"), 0o644))
	status, err = MachineGitSlotStatus(root, slot)
	require.NoError(t, err)
	require.Equal(t, "install", status)
	require.NoError(t, os.MkdirAll(filepath.Dir(slot), 0o755))
	require.NoError(t, os.WriteFile(slot, []byte("tracked"), 0o644))
	output, err = exec.Command("git", "-C", root, "add", "-f", ".agents/skills/review").CombinedOutput()
	require.NoError(t, err, string(output))
	status, err = MachineGitSlotStatus(root, slot)
	require.NoError(t, err)
	require.Equal(t, "conflict", status)
	_, err = MachineGitSlotStatus(root, filepath.Join(root, "..", "outside"))
	require.Error(t, err)
}
