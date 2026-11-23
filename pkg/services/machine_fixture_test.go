package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

type machineFixture struct {
	svc    *Services
	root   string
	main   string
	linked string
	user   string
}

func newMachineFixture(t *testing.T) machineFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := machineFixture{root: filepath.Join(base, "library"), main: filepath.Join(base, "main"), linked: filepath.Join(base, "linked"), user: filepath.Join(base, "user")}
	skill := filepath.Join(f.root, "skills", "local", "review")
	require.NoError(t, os.MkdirAll(skill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: review\ndescription: test\n---\n"), 0o644))
	run := func(args ...string) {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	run("init", "-q", f.main)
	run("-C", f.main, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "initial")
	run("-C", f.main, "worktree", "add", "--detach", "-q", f.linked)
	require.NoError(t, os.MkdirAll(f.user, 0o755))
	f.svc, err = New(&config.Config{Root: f.root, ConfigDir: filepath.Join(base, "config"), Targets: []common.InstallTarget{skillTarget("user-client", f.user)}}, common.NewLogger(false))
	require.NoError(t, err)
	require.NoError(t, f.svc.RegisterDirectories(context.Background(), map[string]string{"main": f.main, "linked": f.linked}, []string{"team"}))
	projectSkills := filepath.Join(f.main, ".agents", "skills")
	require.NoError(t, os.MkdirAll(projectSkills, 0o755))
	require.NoError(t, os.Symlink(skill, filepath.Join(projectSkills, "review")))
	require.NoError(t, os.Symlink(filepath.Join(base, "missing"), filepath.Join(projectSkills, "broken")))
	external := filepath.Join(f.main, ".claude", "skills", "external")
	require.NoError(t, os.MkdirAll(external, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("---\nname: external\ndescription: test\n---\n"), 0o644))
	require.NoError(t, os.Symlink(skill, filepath.Join(f.user, "review")))
	return f
}

func TestMachineFixtureIncludesIndependentCheckoutsAndDiskStates(t *testing.T) {
	f := newMachineFixture(t)
	items, err := f.svc.ScanMachineDirectory("main")
	require.NoError(t, err)
	statuses := map[string]int{}
	for _, item := range items {
		statuses[item.Status]++
	}
	require.Equal(t, 1, statuses["installed"])
	require.Equal(t, 1, statuses["dangling"])
	require.Equal(t, 1, statuses["external"])
	linked, err := f.svc.ScanMachineDirectory("linked")
	require.NoError(t, err)
	require.Empty(t, linked)
	user, err := f.svc.ScanMachineUser()
	require.NoError(t, err)
	require.Len(t, user, 1)
	require.Equal(t, "installed", user[0].Status)
}
