package main

import (
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// readFixture builds the multi-entry repo used by the US2 goldens.
func readFixture(t *testing.T) (root, cfgDir string) {
	t.Helper()
	root = t.TempDir()
	writeTestFile(t, root, "skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: A skill\n---\nbody\n")
	writeTestFile(t, root, "skills/local/team/skill-b/SKILL.md", "---\nname: skill-b\ndescription: A grouped skill\n---\nbody\n")
	writeTestFile(t, root, "commands/github/cmd-a/command.md", "---\nname: cmd-a\ndescription: A remote command\nversion: 1.2.0\n---\nbody\n")
	writeTestFile(t, root, "commands/github/cmd-a/meta.json", `{"address":"https://github.com/x/y","mode_id":"github"}`)
	writeTestFile(t, root, "commands/local/single.md", "---\ndescription: no name here\n---\nbody\n")
	writeTestFile(t, root, "archived/local/old-skill/SKILL.md", "---\nname: old-skill\ndescription: An archived skill\n---\nbody\n")
	writeTestFile(t, root, "skills/local/bad-marker/SKILL.md", "---\ndescription: missing name\n---\nbody\n")
	cfgDir = t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")
	return root, cfgDir
}

// normalizeRoot replaces the (random) temp repo root with the $ROOT sentinel
// used inside the committed goldens.
func normalizeRoot(got, root string) []byte {
	return []byte(strings.ReplaceAll(got, root, "$ROOT"))
}

func TestListJSONGolden(t *testing.T) {
	root, cfgDir := readFixture(t)
	out, err := runCmd(t, "list", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	assertGoldenJSON(t, normalizeRoot(out, root), "../../testdata/golden/list.json")
}

func TestStatusJSONGolden(t *testing.T) {
	root, cfgDir := readFixture(t)
	out, err := runCmd(t, "status", "cmd-a", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	assertGoldenJSON(t, normalizeRoot(out, root), "../../testdata/golden/status.json")
}

func TestInfoJSONGolden(t *testing.T) {
	root, cfgDir := readFixture(t)
	out, err := runCmd(t, "info", "cmd-a", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	assertGoldenJSON(t, normalizeRoot(out, root), "../../testdata/golden/info.json")
}

func TestVerifyJSONGoldenAndExitCodes(t *testing.T) {
	root, cfgDir := readFixture(t)
	// Strict verify on an inconsistent repo -> report + exit 1.
	out, err := runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json")
	require.Error(t, err)
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0), "strict verify exit 1")
	assertGoldenJSON(t, normalizeRoot(out, root), "../../testdata/golden/verify.json")

	// --no-strict reports the same shape but does not fail the exit.
	_, err = runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json", "--no-strict")
	require.NoError(t, err)
}

func TestVerifyCleanRepoExitsZero(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/local/ok/SKILL.md", "---\nname: ok\ndescription: fine\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")
	_, err := runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir)
	require.NoError(t, err, "clean repo verify exits 0")
}

// TestListAndVerifyFlagNonStandardLocation: a skill marker placed outside
// skills/commands/archived is scanned (not invisible) and shows up in both
// `list --json` (status: non_standard) and `verify repo --json` (counted and
// listed as an inconsistency, failing strict verify).
func TestListAndVerifyFlagNonStandardLocation(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "skills/local/ok/SKILL.md", "---\nname: ok\ndescription: fine\n---\nbody\n")
	writeTestFile(t, root, "stray-skill/SKILL.md", "---\nname: stray\ndescription: misplaced\n---\nbody\n")
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json", "[]")

	out, err := runCmd(t, "list", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"status":"non_standard"`)

	out, err = runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json")
	require.Error(t, err, "a non-standard entry fails strict verify")
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0))
	require.Contains(t, out, `"non_standard":1`)
	require.Contains(t, out, `"name":"stray"`)
}

func TestReadCommandStdoutIsJSONOnly(t *testing.T) {
	root, cfgDir := readFixture(t)
	out, err := runCmd(t, "list", "--root", root, "--config", cfgDir, "--json", "--timing")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "{"), "stdout must begin with the JSON object, got: %q", out)
	assertGoldenJSON(t, normalizeRoot(out, root), "../../testdata/golden/list.json")
}
