package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachineScanPathKeepsLibraryRootSeparate(t *testing.T) {
	root, cfg := cmdFixture(t)
	project := filepath.Join(t.TempDir(), "project")
	output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
	require.NoError(t, err, string(output))
	link := filepath.Join(project, ".agents", "skills", "skill-a")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "skills", "local", "skill-a"), link))
	response, err := runCmd(t, "scan", "--path", project, "--skill", "skill-a", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var report struct {
		Items []struct {
			Scope    string `json:"scope"`
			Skill    string `json:"skill"`
			Declared bool   `json:"declared"`
		} `json:"items"`
		Diagnostics []any `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &report))
	require.Len(t, report.Items, 1)
	require.Equal(t, "path", report.Items[0].Scope)
	require.Equal(t, "skills/local/skill-a", report.Items[0].Skill)
	require.False(t, report.Items[0].Declared)
	require.Empty(t, report.Diagnostics)
}
