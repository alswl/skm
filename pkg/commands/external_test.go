package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func externalFixture(t *testing.T) (root, cfgDir, externalPath string) {
	t.Helper()
	root = t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "target")
	externalPath = filepath.Join(targetDir, "external")
	writeTestFile(t, externalPath, "SKILL.md", "---\nname: external\ndescription: external skill\n---\nbody\n")
	cfgDir = t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", `[{"name":"t","path":"`+targetDir+`","kind":"skill"}]`)
	return root, cfgDir, externalPath
}

func TestAdoptCommandCreatesManagedInstall(t *testing.T) {
	root, cfgDir, externalPath := externalFixture(t)
	out, err := runCmd(t, "adopt", externalPath, "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"action":"adopt"`)
	require.Contains(t, out, `"name":"external"`)
	require.FileExists(t, filepath.Join(root, "skills", "local", "external", "SKILL.md"))
	info, err := os.Lstat(externalPath)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestAdoptCommandDryRunDoesNotWrite(t *testing.T) {
	root, cfgDir, externalPath := externalFixture(t)
	out, err := runCmd(t, "adopt", externalPath, "--root", root, "--config", cfgDir, "--dry-run", "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"action":"adopt"`)
	require.Contains(t, out, `"dry_run":true`)
	require.NoDirExists(t, filepath.Join(root, "skills", "local", "external"))
	info, err := os.Lstat(externalPath)
	require.NoError(t, err)
	require.Zero(t, info.Mode()&os.ModeSymlink, "dry-run must not replace the external directory")
}

func TestAdoptCommandAcceptsMultipleExternalSkills(t *testing.T) {
	root, cfgDir, first := externalFixture(t)
	second := filepath.Join(filepath.Dir(first), "second")
	writeTestFile(t, second, "SKILL.md", "---\nname: second\ndescription: second external skill\n---\nbody\n")

	out, err := runCmd(t, "adopt", first, second, "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"name":"external"`)
	require.Contains(t, out, `"name":"second"`)
	require.FileExists(t, filepath.Join(root, "skills", "local", "external", "SKILL.md"))
	require.FileExists(t, filepath.Join(root, "skills", "local", "second", "SKILL.md"))
}

func TestDeleteExternalCommandRequiresForceAndSupportsDryRun(t *testing.T) {
	root, cfgDir, externalPath := externalFixture(t)
	_, err := runCmd(t, "delete-external", externalPath, "--root", root, "--config", cfgDir)
	require.Error(t, err)

	_, err = runCmd(t, "delete-external", externalPath, "--root", root, "--config", cfgDir, "--force", "--dry-run")
	require.NoError(t, err)
	require.DirExists(t, externalPath)

	_, err = runCmd(t, "delete-external", externalPath, "--root", root, "--config", cfgDir, "--force")
	require.NoError(t, err)
	_, statErr := os.Lstat(externalPath)
	require.True(t, os.IsNotExist(statErr))
}

func TestDeleteExternalCommandRefusesManagedSymlink(t *testing.T) {
	root, cfgDir, externalPath := externalFixture(t)
	managed := filepath.Join(root, "skills", "local", "managed")
	writeTestFile(t, managed, "SKILL.md", "---\nname: managed\ndescription: managed skill\n---\nbody\n")
	require.NoError(t, os.RemoveAll(externalPath))
	require.NoError(t, os.Symlink(managed, externalPath))

	_, err := runCmd(t, "delete-external", externalPath, "--root", root, "--config", cfgDir, "--force")
	require.Error(t, err)
	info, statErr := os.Lstat(externalPath)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "a managed symlink must never be deleted by this command")
}

func TestNormalizeCommandMovesNonStandardEntry(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "misplaced/SKILL.md", "---\nname: misplaced\ndescription: misplaced skill\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	out, err := runCmd(t, "normalize", "misplaced", "--root", root, "--config", cfgDir, "--provider", "local", "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"action":"normalize"`)
	require.FileExists(t, filepath.Join(root, "skills", "local", "misplaced", "SKILL.md"))
}

func TestNormalizeCommandDryRunOnlyPreviewsDestination(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "misplaced/SKILL.md", "---\nname: misplaced\ndescription: misplaced skill\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	out, err := runCmd(t, "normalize", "misplaced", "--root", root, "--config", cfgDir, "--dry-run", "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"dry_run":true`)
	require.FileExists(t, filepath.Join(root, "misplaced", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(root, "skills", "local", "misplaced"))
}
