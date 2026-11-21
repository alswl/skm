//go:build darwin || linux

package dal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateMachineLink(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	rel := filepath.Join(".agents", "skills", "review")
	require.NoError(t, CreateMachineLink(root, rel, source))
	link := filepath.Join(root, rel)
	got, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, source, got)
	require.Error(t, CreateMachineLink(root, rel, t.TempDir()))
	got, err = os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, source, got)
	require.Error(t, CreateMachineLink(root, "../escape", source))
}

func TestCreateMachineLinkConcurrentDestination(t *testing.T) {
	root := t.TempDir()
	sources := []string{t.TempDir(), t.TempDir()}
	results := make([]error, len(sources))
	var wg sync.WaitGroup
	for i := range sources {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = CreateMachineLink(root, filepath.Join("skills", "review"), sources[i])
		}(i)
	}
	wg.Wait()
	require.NotEqual(t, results[0] == nil, results[1] == nil)
	target, err := os.Readlink(filepath.Join(root, "skills", "review"))
	require.NoError(t, err)
	if results[0] == nil {
		require.Equal(t, sources[0], target)
	} else {
		require.Equal(t, sources[1], target)
	}
}

func TestCreateMachineLinkRejectsSymlinkParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, ".agents")))
	require.Error(t, CreateMachineLink(root, filepath.Join(".agents", "skills", "review"), t.TempDir()))
	_, err := os.Lstat(filepath.Join(outside, "skills"))
	require.True(t, os.IsNotExist(err))
}
