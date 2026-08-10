package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitializeRepositoryCreatesSkillsTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my-skills")
	got, err := InitializeRepository(root)
	require.NoError(t, err)
	require.Equal(t, root, got)
	require.DirExists(t, filepath.Join(root, "skills"))
}

func TestInitializeRepositoryRefusesNonEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o644))
	_, err := InitializeRepository(root)
	require.ErrorContains(t, err, "not empty")
	require.FileExists(t, filepath.Join(root, "keep.txt"))
}
