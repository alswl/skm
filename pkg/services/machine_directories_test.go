package services

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineDirectoryRegistry(t *testing.T) {
	library := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	cmd := exec.Command("git", "init", "-q", project)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	svc := &Services{Cfg: &config.Config{Root: library, ConfigDir: t.TempDir()}}
	ctx := context.Background()
	require.NoError(t, svc.RegisterDirectories(ctx, map[string]string{"project": project}, []string{"go"}))
	require.NoError(t, svc.RegisterDirectories(ctx, map[string]string{"project": project}, []string{"go"}))
	require.NoError(t, svc.TagDirectories(ctx, []string{"project"}, []string{"team"}, []string{"go"}))
	entries, err := svc.ListDirectories()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "project", entries[0].ID)
	require.Equal(t, []string{"team"}, entries[0].Tags)
	require.Error(t, svc.TagDirectories(ctx, []string{"missing"}, []string{"x"}, nil))
	require.Error(t, svc.RegisterDirectories(ctx, map[string]string{"duplicate": project}, nil))
	newProject := filepath.Join(t.TempDir(), "new-project")
	output, err = exec.Command("git", "init", "-q", newProject).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, svc.SetDirectoryPath(ctx, "project", newProject))
	entries, err = svc.ListDirectories()
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(newProject)
	require.NoError(t, err)
	require.Equal(t, canonical, entries[0].Path)
	require.Error(t, svc.SetDirectoryPath(ctx, "missing", newProject))
}

func TestMachineDirectoryRegistrationRetriesAfterPathWriteFailure(t *testing.T) {
	library, configDir := t.TempDir(), t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	svc := &Services{Cfg: &config.Config{Root: library, ConfigDir: configDir}}
	err = svc.registerDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}, func(path string, value any, mode os.FileMode) error {
		if path != filepath.Join(library, "installations.yaml") {
			return errors.New("injected local path failure")
		}
		return config.WriteYAMLAtomic(path, value, mode)
	})
	require.ErrorContains(t, err, "directories registered, but local paths could not be saved")
	doc, err := config.LoadInstallations(filepath.Join(library, "installations.yaml"))
	require.NoError(t, err)
	require.Equal(t, []string{"team"}, doc.Directories["project"].Tags)
	require.NoError(t, svc.RegisterDirectories(context.Background(), map[string]string{"project": project}, []string{"team"}))
	items, err := svc.ListDirectories()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEmpty(t, items[0].Path)
}

func TestMachineDirectoryBatchValidationWritesNothing(t *testing.T) {
	library, configDir := t.TempDir(), t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	svc := &Services{Cfg: &config.Config{Root: library, ConfigDir: configDir}}
	err = svc.RegisterDirectories(context.Background(), map[string]string{"valid": project, "INVALID": project}, []string{"team"})
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(library, "installations.yaml"))
}
