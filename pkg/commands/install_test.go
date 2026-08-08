package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

// cmdFixture builds a repository + config dir + a single target for CLI
// integration tests.
func cmdFixture(t *testing.T, kind string) (root, cfgDir string) {
	t.Helper()
	root = t.TempDir()
	writeTestFile(t, root, "skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: A skill\n---\nbody\n")

	cfgDir = filepath.Join(t.TempDir(), "cfg")
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	cfg := `[{"name":"t","path":"` + targetDir + `","builtin":false,"kind":"` + kind + `"}]`
	writeTestFile(t, cfgDir, "targets.json", cfg)
	return root, cfgDir
}

func writeTestFile(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

// runCmd executes the root command with fresh buffers and reset global flags,
// because pflag keeps last-parsed values in the bound vars between Execute
// calls. Stdout is captured separately from stderr so JSON purity is
// assertable.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	installTargets = nil
	flagRoot, flagConfig = "", ""
	flagJSON, flagTiming, flagDryRun, flagForce = false, false, false, false
	flagNoStrict = false
	resetTargetFlags()
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return outBuf.String(), err
}

// assertGoldenJSON compares got (a JSON document) semantically against a
// committed golden file.
func assertGoldenJSON(t *testing.T, got []byte, goldenPath string) {
	t.Helper()
	var want, g any
	require.NoError(t, json.Unmarshal(got, &g))
	data, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &want))
	require.Equal(t, want, g)
}

// fixtureTargetDir extracts the target path from a fixture targets.json.
func fixtureTargetDir(t *testing.T, cfgDir string) string {
	t.Helper()
	var targets []struct{ Path string }
	data, err := os.ReadFile(filepath.Join(cfgDir, "targets.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &targets))
	require.NotEmpty(t, targets)
	return targets[0].Path
}

func TestInstallCommandJSONGolden(t *testing.T) {
	root, cfgDir := cmdFixture(t, "skill")
	out, err := runCmd(t, "install", "skill-a", "--root", root, "--config", cfgDir, "--json", "--target", "t")
	require.NoError(t, err)
	assertGoldenJSON(t, []byte(out), "../../testdata/golden/install.json")
}

func TestInstallCommandDryRunJSONGolden(t *testing.T) {
	root, cfgDir := cmdFixture(t, "skill")
	targetDir := fixtureTargetDir(t, cfgDir)
	out, err := runCmd(t, "install", "skill-a", "--root", root, "--config", cfgDir, "--json", "--dry-run", "--target", "t")
	require.NoError(t, err)
	assertGoldenJSON(t, []byte(out), "../../testdata/golden/install-dryrun.json")
	// Dry-run must not write anything (SC-007).
	_, statErr := os.Lstat(filepath.Join(targetDir, "skill-a"))
	require.True(t, os.IsNotExist(statErr), "dry-run must not create the link")
}

func TestUninstallCommandJSONGolden(t *testing.T) {
	root, cfgDir := cmdFixture(t, "skill")
	// Install first so uninstall has something managed to remove.
	_, err := runCmd(t, "install", "skill-a", "--root", root, "--config", cfgDir, "--target", "t")
	require.NoError(t, err)
	out, err := runCmd(t, "uninstall", "skill-a", "--root", root, "--config", cfgDir, "--json", "--target", "t")
	require.NoError(t, err)
	assertGoldenJSON(t, []byte(out), "../../testdata/golden/uninstall.json")
}

func TestInstallRefusesConflictExitCode(t *testing.T) {
	root, cfgDir := cmdFixture(t, "skill")
	// A user file at the target blocks the install (conflict).
	targetDir := fixtureTargetDir(t, cfgDir)
	writeTestFile(t, targetDir, "skill-a", "user data")

	_, err := runCmd(t, "install", "skill-a", "--root", root, "--config", cfgDir, "--target", "t")
	require.Error(t, err, "conflict must be refused")
	require.Equal(t, common.ExitObject, common.ExitCodeOf(err, 0), "conflict is an object problem -> exit 1")
}
