package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetsBootstrapCreatesAndConverges(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	bin := filepath.Join(t.TempDir(), "skm")
	build := exec.Command("go", "build", "-o", bin, "./cmd/skm")
	build.Dir = repo
	require.NoError(t, build.Run())
	root := t.TempDir()
	cfg, dest := filepath.Join(root, "config"), filepath.Join(root, "skills with spaces")
	env := append(os.Environ(), "PATH="+filepath.Dir(bin)+":"+os.Getenv("PATH"), "HOME="+filepath.Join(root, "home"), "XDG_CONFIG_HOME="+filepath.Join(root, "xdg"))
	run := func() (string, error) {
		c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "acme", dest, "--config", cfg)
		c.Env = env
		out, err := c.CombinedOutput()
		return string(out), err
	}
	out, err := run()
	require.NoError(t, err, out)
	require.Contains(t, out, "added acme")
	require.Contains(t, out, "added acme-commands")
	first, err := os.ReadFile(filepath.Join(cfg, "config.yaml"))
	require.NoError(t, err)
	writeTestFile(t, root, "repo/skills/local/skill-a/SKILL.md", "---\nname: skill-a\ndescription: test\n---\nbody\n")
	writeTestFile(t, root, "repo/commands/local/command-a/command.md", "---\nname: command-a\ndescription: test\n---\nbody\n")
	install := func(name, target string) {
		c := exec.Command(bin, "install", name, "--root", filepath.Join(root, "repo"), "--config", cfg, "--target", target)
		c.Env = env
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	install("skill-a", "acme")
	install("command-a", "acme-commands")
	require.FileExists(t, filepath.Join(dest, "skill-a", "SKILL.md"))
	require.FileExists(t, filepath.Join(dest, "command-a", ".skm-adapter"))
	out, err = run()
	require.NoError(t, err, out)
	require.Contains(t, out, "unchanged acme")
	second, err := os.ReadFile(filepath.Join(cfg, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.DirExists(t, dest)
}

func TestTargetsBootstrapRejectsBadInput(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "bad/name", t.TempDir())
	out, err := c.CombinedOutput()
	require.Error(t, err)
	require.True(t, strings.Contains(string(out), "invalid base name"))
}

func TestTargetsBootstrapReportsFinalListingFailureAndRetry(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	root := t.TempDir()
	real := filepath.Join(root, "real-skm")
	build := exec.Command("go", "build", "-o", real, "./cmd/skm")
	build.Dir = repo
	require.NoError(t, build.Run())
	bin := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	count := filepath.Join(root, "count")
	shim := "#!/bin/sh\nn=$(cat \"$SKM_BOOTSTRAP_COUNT\" 2>/dev/null || echo 0)\nn=$((n+1))\necho \"$n\" > \"$SKM_BOOTSTRAP_COUNT\"\nif [ \"$n\" -eq 4 ]; then echo final-list-failure >&2; exit 1; fi\nexec \"$SKM_BOOTSTRAP_REAL\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "skm"), []byte(shim), 0o755))
	cfg, dest := filepath.Join(root, "config"), filepath.Join(root, "skills")
	env := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "SKM_BOOTSTRAP_COUNT="+count, "SKM_BOOTSTRAP_REAL="+real, "HOME="+filepath.Join(root, "home"), "XDG_CONFIG_HOME="+filepath.Join(root, "xdg"))
	run := func() (string, error) {
		c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "acme", dest, "--config", cfg)
		c.Env = env
		out, err := c.CombinedOutput()
		return string(out), err
	}
	out, err := run()
	require.Error(t, err)
	require.Contains(t, out, "final target listing failed")
	require.FileExists(t, filepath.Join(cfg, "config.yaml"))
	require.NoError(t, os.Remove(count))
	out, err = run()
	require.NoError(t, err, out)
	require.Contains(t, out, "unchanged acme")
}

func TestTargetsBootstrapMigratesCustomTargetAndPreservesSettings(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	bin := filepath.Join(t.TempDir(), "skm")
	build := exec.Command("go", "build", "-o", bin, "./cmd/skm")
	build.Dir = repo
	require.NoError(t, build.Run())
	root := t.TempDir()
	cfg, dest := filepath.Join(root, "config"), filepath.Join(root, "skills")
	writeTestFile(t, cfg, "config.yaml", "plugin_dirs:\n  - /tmp/unrelated\ntargets:\n  - name: acme\n    platform: legacy\n    path: /tmp/legacy\n    accepts: [skill]\n    strategies:\n      skill: plugin:acme\n    name_rule: kebab-case\n  - name: unrelated\n    path: /tmp/unrelated\n    accepts: [skill]\n    strategies:\n      skill: skill-symlink\n")
	env := append(os.Environ(), "PATH="+filepath.Dir(bin)+":"+os.Getenv("PATH"), "HOME="+filepath.Join(root, "home"), "XDG_CONFIG_HOME="+filepath.Join(root, "xdg"))
	c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "acme", dest, "--config", cfg)
	c.Env = env
	out, err := c.CombinedOutput()
	require.NoError(t, err, string(out))
	data, err := os.ReadFile(filepath.Join(cfg, "config.yaml"))
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "name_rule: kebab-case")
	require.Contains(t, text, "name: unrelated")
	require.Contains(t, text, "command-adapter")
	require.NotContains(t, text, "plugin:acme")
}

// An option token in a positional slot is a missing positional, not a base
// name: `--config <dir>` alone used to parse as base=--config with the
// --config swallowed, so the bogus records landed in the default config dir.
func TestTargetsBootstrapRejectsOptionAsPositional(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	root := t.TempDir()
	home, cfg := filepath.Join(root, "home"), filepath.Join(root, "config")
	c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "--config", cfg)
	c.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(root, "xdg"))
	out, err := c.CombinedOutput()
	require.Error(t, err, string(out))
	require.Contains(t, string(out), "missing <base-name>")
	require.NotContains(t, string(out), "added")
	require.NoDirExists(t, cfg)
	require.NoDirExists(t, filepath.Join(root, "xdg", "skm"))
}

// The update path reports `updated <name>`, per contract #results — it printed
// "updateed" from a "${action}ed" interpolation.
func TestTargetsBootstrapReportsUpdatedWording(t *testing.T) {
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	bin := filepath.Join(t.TempDir(), "skm")
	build := exec.Command("go", "build", "-o", bin, "./cmd/skm")
	build.Dir = repo
	require.NoError(t, build.Run())
	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	env := append(os.Environ(), "PATH="+filepath.Dir(bin)+":"+os.Getenv("PATH"), "HOME="+filepath.Join(root, "home"), "XDG_CONFIG_HOME="+filepath.Join(root, "xdg"))
	run := func(dest string) string {
		c := exec.Command(filepath.Join(repo, "scripts", "skm-targets-bootstrap.sh"), "acme", dest, "--config", cfg)
		c.Env = env
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	require.Contains(t, run(filepath.Join(root, "first")), "added acme")
	out := run(filepath.Join(root, "second"))
	require.Contains(t, out, "updated acme")
	require.Contains(t, out, "updated acme-commands")
	require.NotContains(t, out, "updateed")
}
