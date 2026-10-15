package providers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionedFetchCapturesActualCommitAndHistoricalTree(t *testing.T) {
	source := t.TempDir()
	run := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", source}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "skills/demo"), 0700))
	file := filepath.Join(source, "skills/demo/SKILL.md")
	require.NoError(t, os.WriteFile(file, []byte("old"), 0600))
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "old")
	old := run("rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(file, []byte("new"), 0600))
	run("add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "new")
	provider := gitHostProvider{id: "test", host: "example.invalid"}
	got, err := provider.FetchVersioned(context.Background(), FetchRequest{RepositoryAddress: source, Subpath: "skills/demo"})
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(got.Path) }()
	require.Equal(t, run("rev-parse", "HEAD"), got.Revision)
	require.Equal(t, "skills/demo", got.ResolvedSubpath)
	historical, err := provider.FetchVersioned(context.Background(), FetchRequest{RepositoryAddress: source, Subpath: "skills/demo", Revision: old})
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(historical.Path) }()
	data, err := os.ReadFile(filepath.Join(historical.Path, historical.ResolvedSubpath, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "old", string(data))
	require.Equal(t, old, historical.Revision)
	_, err = provider.FetchVersioned(context.Background(), FetchRequest{RepositoryAddress: source, Subpath: "../escape"})
	require.Error(t, err)
}
