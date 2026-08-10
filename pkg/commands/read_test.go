package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestVerifyArchivedSameNameAsActiveIsNotDanglingOrConflict: when an entry is
// archived and a new version is reinstalled under the same name, the archived
// copy must neither be reported as a dangling install (the target link named
// demo is the active entry's healthy install) nor as a name conflict —
// archiving old + installing new under the same name is a normal workflow
// (only StatusActive entries can be installed; models.go). Regression for the
// phantom dangling/name_conflict on archived entries.
func TestVerifyArchivedSameNameAsActiveIsNotDanglingOrConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // hermetic: the merged-in built-in targets point into an empty home
	root := t.TempDir()
	writeTestFile(t, root, "skills/local/demo/SKILL.md", "---\nname: demo\ndescription: new version\n---\nbody\n")
	writeTestFile(t, root, "archived/local/demo/SKILL.md", "---\nname: demo\ndescription: old archived version\n---\nbody\n")
	targetDir := t.TempDir()
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json",
		`[{"name":"t","path":"`+targetDir+`","accepts":["skill"],"strategies":{"skill":"skill-symlink"}}]`)
	// The active entry is healthy-installed: the link named demo resolves to
	// the active path, not the archived one.
	require.NoError(t, os.Symlink(filepath.Join(root, "skills/local/demo"), filepath.Join(targetDir, "demo")))

	out, err := runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err, "active+archived same name with a healthy install must verify clean")
	var rep verifyReport
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	require.True(t, rep.Consistent)
	require.Empty(t, rep.Inconsistencies)
	require.Empty(t, rep.NameConflicts)
	require.Equal(t, 1, rep.Active)
	require.Equal(t, 1, rep.Archived)
}

// TestVerifyFlatArchivedSameNameAsActiveIsNotDanglingOrConflict: an archived
// copy stored flat at archived/<name>/SKILL.md (no provider level) is still an
// archived entry — a marker under the archived/ tree is StatusArchived, not
// non-standard (models.go reserves NonStandard for markers outside the managed
// skills/commands/archived trees). It must neither be reported as a dangling
// install (the target link named demo is the active entry's healthy install)
// nor as a name conflict, exactly like a properly-nested archived copy.
// Regression for flat archived entries leaking into verify's dangling and
// name-conflict checks.
func TestVerifyFlatArchivedSameNameAsActiveIsNotDanglingOrConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // hermetic: the merged-in built-in targets point into an empty home
	root := t.TempDir()
	writeTestFile(t, root, "skills/local/demo/SKILL.md", "---\nname: demo\ndescription: new version\n---\nbody\n")
	writeTestFile(t, root, "archived/demo/SKILL.md", "---\nname: demo\ndescription: old archived version\n---\nbody\n")
	targetDir := t.TempDir()
	cfgDir := t.TempDir()
	writeTestFile(t, cfgDir, "targets.json",
		`[{"name":"t","path":"`+targetDir+`","accepts":["skill"],"strategies":{"skill":"skill-symlink"}}]`)
	// The active entry is healthy-installed: the link named demo resolves to
	// the active path, not the archived one.
	require.NoError(t, os.Symlink(filepath.Join(root, "skills/local/demo"), filepath.Join(targetDir, "demo")))

	out, err := runCmd(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json")
	require.NoError(t, err, "flat archived + same-name active with a healthy install must verify clean")
	var rep verifyReport
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	require.True(t, rep.Consistent)
	require.Empty(t, rep.Inconsistencies)
	require.Empty(t, rep.NameConflicts)
	require.Equal(t, 1, rep.Active)
	require.Equal(t, 1, rep.Archived)
}
