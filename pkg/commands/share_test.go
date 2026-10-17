package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func extractPayloadCommand(t *testing.T, createOutput string) string {
	t.Helper()
	start := strings.Index(createOutput, "'")
	end := strings.LastIndex(createOutput, "'")
	require.True(t, start >= 0 && end > start, "unexpected share create output: %q", createOutput)
	return createOutput[start+1 : end]
}

func TestShareCreateEmitsApplyCommand(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	writeTestFile(t, root, "skills/local/skill-a/meta.json", `{"address":"https://github.com/acme/skill-a","mode_id":"local"}`)

	out, err := runCmd(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, out, "skm share apply 'skm1z")
}

func TestShareCreateReportsProgressForMultipleEntriesOnly(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	writeTestFile(t, root, "skills/local/skill-a/meta.json", `{"address":"https://github.com/acme/skill-a","mode_id":"local"}`)
	writeTestFile(t, root, "skills/local/skill-b/SKILL.md", "---\nname: skill-b\ndescription: B skill\n---\nbody\n")
	writeTestFile(t, root, "skills/local/skill-b/meta.json", `{"address":"https://github.com/acme/skill-b","mode_id":"local"}`)

	_, stderr, err := runCmdWithStderr(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Empty(t, stderr, "a single selected entry should not print progress")

	_, stderr, err = runCmdWithStderr(t, "share", "create", "skill-a", "skill-b", "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, stderr, "share: 1/2")
	require.Contains(t, stderr, "share: 2/2")
}

func TestShareCreateJSONIncludesEntries(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	writeTestFile(t, root, "skills/local/skill-a/meta.json", `{"address":"https://github.com/acme/skill-a","mode_id":"local"}`)

	out, err := runCmd(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"name":"skill-a"`)
	require.Contains(t, out, `"source":"https://github.com/acme/skill-a"`)
}

func TestShareCreateRejectsLocalOnlyExplicitEntry(t *testing.T) {
	root, cfgDir := cmdFixture(t) // skill-a has no origin and the fixture is not a git repo
	_, err := runCmd(t, "share", "create", "skill-a", "--root", root, "--config", cfgDir)
	require.Error(t, err)
}

func TestShareApplyRejectsMalformedPayload(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	_, err := runCmd(t, "share", "apply", "not-a-payload", "--root", root, "--config", cfgDir)
	require.Error(t, err)
}

func TestShareApplySkipsSameNameCollisionWithoutFetching(t *testing.T) {
	srcRoot, srcCfgDir := cmdFixture(t)
	writeTestFile(t, srcRoot, "skills/local/skill-a/meta.json", `{"address":"https://github.com/acme/skill-a","mode_id":"local"}`)
	create, err := runCmd(t, "share", "create", "skill-a", "--root", srcRoot, "--config", srcCfgDir)
	require.NoError(t, err)
	payload := extractPayloadCommand(t, create)

	// The destination already has its own skill-a, so apply must skip it
	// without ever fetching the (unreachable in this test) source address.
	destRoot, destCfgDir := cmdFixture(t)
	out, err := runCmd(t, "share", "apply", payload, "--root", destRoot, "--config", destCfgDir)
	require.Error(t, err)
	require.Contains(t, out, "skipped")
}
