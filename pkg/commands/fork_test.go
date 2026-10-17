package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

func TestForkCLIWorkflowAndFailedJSON(t *testing.T) {
	root, cfg := cmdFixture(t)
	source := t.TempDir()
	writeTestFile(t, source, "SKILL.md", "---\nname: review\ndescription: review\n---\nbase\n")
	call := func(args ...string) (string, error) {
		return runCmd(t, append(args, "--root", root, "--config", cfg, "--json")...)
	}
	_, err := call("import", source)
	require.NoError(t, err)
	out, err := call("fork", "review")
	require.NoError(t, err)
	var fork engines.UpdateResult
	require.NoError(t, json.Unmarshal([]byte(out), &fork))
	require.True(t, fork.Accepted)
	require.Equal(t, filepath.Join(root, "skills-fork", "local", "review"), fork.ActivePath)
	require.FileExists(t, filepath.Join(fork.ActivePath, "SKILL.md"))
	writeTestFile(t, fork.ActivePath, "SKILL.md", "---\nname: review\ndescription: review\n---\nlocal\n")
	writeTestFile(t, source, "SKILL.md", "---\nname: review\ndescription: review\n---\nupstream\n")
	out, err = call("update", "review")
	require.Error(t, err)
	var failed engines.UpdateResult
	require.NoError(t, json.Unmarshal([]byte(out), &failed))
	require.Equal(t, "manual_required", failed.Outcome)
	require.NotEmpty(t, failed.AttemptPath)
	_, err = call("fork", "review", "--upstream")
	require.Error(t, err)
	_, err = call("update", "review", "--abort")
	require.NoError(t, err)
	require.NoDirExists(t, failed.AttemptPath)
	_, err = call("fork", "review", "--upstream")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(fork.ActivePath, "SKILL.md"))
	require.NoError(t, err)
	_, err = call("fork", "review")
	require.Error(t, err)
	_, err = call("fork", "review", "--replace")
	require.NoError(t, err)
	_, err = call("update", "review", "--accept", "--abort")
	require.Error(t, err)
}

func TestSyncMigrateCLIFlagsAndCurrentResult(t *testing.T) {
	root, cfg := cmdFixture(t)
	_, err := runCmd(t, "sync", "migrate", "--accept", "--rollback", "--root", root, "--config", cfg)
	require.Error(t, err)
	out, err := runCmd(t, "sync", "migrate", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var result engines.MigrationResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "current", result.Outcome)
}
