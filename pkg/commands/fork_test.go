package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/stretchr/testify/require"
)

func TestForkCLIWorkflow(t *testing.T) {
	root, cfg := cmdFixture(t)
	source := t.TempDir()
	body := "l1\nl2\nl3\nl4\nl5\n"
	writeTestFile(t, source, "SKILL.md", "---\nname: review\ndescription: review\n---\n"+body)
	call := func(args ...string) (string, error) {
		return runCmd(t, append(args, "--root", root, "--config", cfg, "--json")...)
	}
	forkDir := filepath.Join(root, "skills-fork", "local", "review")
	upstream := filepath.Join(root, "skills", "local", "review")

	_, err := call("import", source)
	require.NoError(t, err)
	out, err := call("fork", "review")
	require.NoError(t, err)
	var res services.ForkResult
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Equal(t, "forked", res.Action)
	require.Equal(t, forkDir, res.ActivePath)

	out, err = call("info", "review")
	require.NoError(t, err)
	var info infoReport
	require.NoError(t, json.Unmarshal([]byte(out), &info))
	require.Equal(t, forkDir, info.Path)
	require.Equal(t, upstream, info.UpstreamPath)

	_, err = call("fork", "review")
	require.ErrorContains(t, err, "--replace")

	// A clean update carries the fork's change over a new upstream version.
	writeTestFile(t, forkDir, "SKILL.md", "---\nname: review\ndescription: review\n---\nmine\nl2\nl3\nl4\nl5\n")
	writeTestFile(t, source, "SKILL.md", "---\nname: review\ndescription: review\n---\nl1\nl2\nl3\nl4\nupstream\n")
	_, err = call("update", "review")
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(forkDir, "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "mine")
	require.Contains(t, string(data), "upstream")

	// A conflicting update fails, names the file and leaves the fork alone.
	writeTestFile(t, source, "SKILL.md", "---\nname: review\ndescription: review\n---\ntheirs\nl2\nl3\nl4\nupstream\n")
	before, _ := os.ReadFile(filepath.Join(forkDir, "SKILL.md"))
	_, stderr, err := runCmdWithStderr(t, "update", "review", "--root", root, "--config", cfg)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "conflict in SKILL.md") || strings.Contains(stderr, "conflict in SKILL.md"), err.Error())
	after, _ := os.ReadFile(filepath.Join(forkDir, "SKILL.md"))
	require.Equal(t, string(before), string(after))

	out, err = call("batch-update")
	require.NoError(t, err)
	var batch services.BatchUpdateResult
	require.NoError(t, json.Unmarshal([]byte(out), &batch))
	require.Len(t, batch.Failed, 1)
	require.Contains(t, batch.Failed[0].Reason, "conflict in SKILL.md")

	_, err = call("fork", "review", "--replace")
	require.NoError(t, err)
	data, _ = os.ReadFile(filepath.Join(forkDir, "SKILL.md"))
	require.NotContains(t, string(data), "mine")
	_, err = call("fork", "review", "--replace", "--discard")
	require.Error(t, err)
	_, err = call("fork", "review", "--discard")
	require.NoError(t, err)
	require.NoDirExists(t, forkDir)
	_, err = call("fork", "review", "--discard")
	require.ErrorContains(t, err, "no fork")
}
