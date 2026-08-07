package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// E2E CLI suite: builds the real `skm` binary once and runs it as a
// subprocess against the committed testdata/e2e/repo fixture — the exact
// binary the user runs, with real stdout/stderr/exit codes. The fixture is
// copied to a temp dir first so no test mutates the committed files.

var (
	e2eOnce sync.Once
	e2eBin  string
	e2eErr  error
)

func e2eBinary(t *testing.T) string {
	t.Helper()
	e2eOnce.Do(func() {
		dir, err := os.MkdirTemp("", "skm-e2e-bin")
		if err != nil {
			e2eErr = err
			return
		}
		e2eBin = filepath.Join(dir, "skm")
		if out, err := exec.Command("go", "build", "-o", e2eBin, ".").CombinedOutput(); err != nil {
			e2eErr = err
			_ = out
		}
	})
	require.NoError(t, e2eErr, "building the e2e binary")
	return e2eBin
}

// e2eFixture copies the committed testdata/e2e/repo into a fresh temp dir and
// returns (tempRepo, tempCfgDir). The config holds one skill target whose path
// is the test's own temp target dir.
func e2eFixture(t *testing.T) (root, cfgDir string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, copyTestTree(t, "../../testdata/e2e/repo", root))
	cfgDir = t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	writeTestFile(t, cfgDir, "targets.json",
		`[{"name":"claude-skills","path":"`+targetDir+`","builtin":false,"accepts":["skill"],"strategies":{"skill":"skill-symlink"}}]`)
	return root, cfgDir
}

// copyTestTree copies the committed fixture dir into dst.
func copyTestTree(t *testing.T, src, dst string) error {
	t.Helper()
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// e2eRun executes the built binary and returns stdout, stderr and exit code.
func e2eRun(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(e2eBinary(t), args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return out.String(), errb.String(), ee.ExitCode()
	}
	require.NoError(t, err, "skm %s", strings.Join(args, " "))
	return out.String(), errb.String(), 0
}

// e2eJSON decodes a --json report into a generic map.
func e2eJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m), "stdout is valid JSON: %s", s)
	return m
}

// TestE2ECLIListAndVerifyReportsNonStandardLocations: `skm list --json` and
// `skm verify repo` report the fixture's three misplaced entries (d2,
// repo-analyzer, kms.log) and the one properly-nested control (proper-skill).
func TestE2ECLIListAndVerifyReportsNonStandardLocations(t *testing.T) {
	root, cfgDir := e2eFixture(t)

	out, _, code := e2eRun(t, "list", "--root", root, "--config", cfgDir, "--json")
	require.Equal(t, 0, code)
	rep := e2eJSON(t, out)
	require.Equal(t, float64(4), rep["total"])
	entries := rep["entries"].([]any)
	names := make([]string, 0, len(entries))
	nonStandard := 0
	for _, e := range entries {
		em := e.(map[string]any)
		names = append(names, em["name"].(string))
		if em["status"] == "non_standard" {
			nonStandard++
		}
	}
	require.Equal(t, []string{"d2", "kms.log", "proper-skill", "repo-analyzer"}, names)
	require.Equal(t, 3, nonStandard, "three misplaced entries are reported non-standard")

	out, _, code = e2eRun(t, "verify", "repo", "--root", root, "--config", cfgDir, "--json")
	require.Equal(t, 1, code, "verify repo fails (non-zero) when the repo is inconsistent")
	v := e2eJSON(t, out)
	require.Equal(t, float64(4), v["total"])
	require.Equal(t, float64(3), v["non_standard"])
	require.Equal(t, float64(1), v["active"])
	require.Equal(t, false, v["consistent"])
}

// TestE2ECLIProviderListAndValidate: the five built-ins are discoverable, and
// validating one succeeds.
func TestE2ECLIProviderListAndValidate(t *testing.T) {
	_, cfgDir := e2eFixture(t)

	out, _, code := e2eRun(t, "provider", "list", "--config", cfgDir, "--json")
	require.Equal(t, 0, code)
	rep := e2eJSON(t, out)
	providers := rep["providers"].([]any)
	ids := make([]string, 0, len(providers))
	for _, p := range providers {
		ids = append(ids, p.(map[string]any)["id"].(string))
	}
	require.Equal(t, []string{"local", "self-build", "github", "gitlab", "skills-sh"}, ids)

	out, _, code = e2eRun(t, "provider", "validate", "gitlab", "--config", cfgDir, "--json")
	require.Equal(t, 0, code)
	v := e2eJSON(t, out)
	require.Equal(t, true, v["success"])
}

// TestE2ECLITargetLifecycle: add/list/validate/update/remove round-trips a
// target through the real binary, and the report tracks the change.
func TestE2ECLITargetLifecycle(t *testing.T) {
	_, cfgDir := e2eFixture(t)

	out, _, _ := e2eRun(t, "target", "list", "--config", cfgDir, "--json")
	require.Len(t, e2eJSON(t, out)["targets"].([]any), 4, "fixture's one claude-skills entry merges with the 3 other built-ins")

	newTarget := filepath.Join(t.TempDir(), "my-tool")
	_, stderr, code := e2eRun(t, "target", "add", "--config", cfgDir,
		"--name", "my-tool", "--platform", "darwin", "--path", newTarget,
		"--accepts", "command", "--strategy", "command=command-marker")
	require.Equal(t, 0, code, "target add fails: %s", stderr)

	out, _, _ = e2eRun(t, "target", "list", "--config", cfgDir, "--json")
	targets := e2eJSON(t, out)["targets"].([]any)
	require.Len(t, targets, 5)
	var myTool map[string]any
	for _, tgt := range targets {
		if tgt.(map[string]any)["name"] == "my-tool" {
			myTool = tgt.(map[string]any)
		}
	}
	require.NotNil(t, myTool)
	require.Equal(t, "darwin", myTool["platform"])
	require.Equal(t, []any{"command"}, myTool["accepts"])

	_, _, code = e2eRun(t, "target", "validate", "my-tool", "--config", cfgDir, "--json")
	require.Equal(t, 0, code)

	newPath := filepath.Join(t.TempDir(), "relocated")
	_, stderr, code = e2eRun(t, "target", "update", "--config", cfgDir, "--name", "my-tool", "--path", newPath)
	require.Equal(t, 0, code, "target update fails: %s", stderr)

	_, _, code = e2eRun(t, "target", "remove", "--config", cfgDir, "--name", "my-tool")
	require.Equal(t, 0, code)

	out, _, _ = e2eRun(t, "target", "list", "--config", cfgDir, "--json")
	require.Len(t, e2eJSON(t, out)["targets"].([]any), 4, "back to the fixture's merged built-ins")
}
