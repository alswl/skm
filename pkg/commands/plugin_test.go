package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/services"
	"github.com/stretchr/testify/require"
)

// writePluginFile creates an executable plugin stub at <dir>/<rel>.
func writePluginFile(t *testing.T, dir, rel string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\nread -r line\necho '{\"id\":\"acme\"}'\n"), 0o755))
	return p
}

// pluginHome isolates the plugin directory, which follows $HOME/$XDG_CONFIG_HOME
// rather than --config, and returns it.
func pluginHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return filepath.Join(home, ".config", "skm", "plugins")
}

func TestPluginAddListRemove(t *testing.T) {
	pluginDir := pluginHome(t)
	cfgDir := t.TempDir()
	// The kind comes from the source's own providers/ directory.
	src := writePluginFile(t, t.TempDir(), "plugins/providers/acme")

	out, err := runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.NoError(t, err)
	var addRep struct {
		Added   services.PluginInfo `json:"added"`
		Success bool                `json:"success"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &addRep))
	require.True(t, addRep.Success)
	require.Equal(t, "provider", addRep.Added.Kind)
	link := filepath.Join(pluginDir, "providers", "acme")
	require.Equal(t, link, addRep.Added.Path)
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, src, dest)

	// Adding the same name again needs --force.
	_, err = runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.ErrorContains(t, err, "--force")
	_, err = runCmd(t, "plugin", "add", src, "--config", cfgDir, "--force", "--json")
	require.NoError(t, err)

	// The linked plugin loads like any other.
	out, err = runCmd(t, "provider", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"acme"`)

	out, err = runCmd(t, "plugin", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	var listRep services.PluginListResult
	require.NoError(t, json.Unmarshal([]byte(out), &listRep))
	require.Len(t, listRep.Plugins, 1)
	require.Equal(t, src, listRep.Plugins[0].Source)
	require.False(t, listRep.Plugins[0].Broken)

	_, err = runCmd(t, "plugin", "remove", "acme", "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.NoFileExists(t, link)
	require.FileExists(t, src, "removing a plugin must not touch the source")

	_, err = runCmd(t, "plugin", "remove", "acme", "--config", cfgDir, "--json")
	require.ErrorContains(t, err, "not found")
}

func TestPluginAddRequiresKindWhenNotInferable(t *testing.T) {
	pluginDir := pluginHome(t)
	cfgDir := t.TempDir()
	src := writePluginFile(t, t.TempDir(), "bin/my-plugin")

	_, err := runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.ErrorContains(t, err, "--kind")

	out, err := runCmd(t, "plugin", "add", src, "--kind", "target", "--name", "codefuse", "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"kind":"target"`)
	require.FileExists(t, filepath.Join(pluginDir, "targets", "codefuse"))
}

func TestPluginAddRejectsNonExecutableAndDirectory(t *testing.T) {
	pluginHome(t)
	cfgDir := t.TempDir()
	dir := t.TempDir()
	plain := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o644))

	_, err := runCmd(t, "plugin", "add", plain, "--kind", "provider", "--config", cfgDir, "--json")
	require.ErrorContains(t, err, "not executable")

	_, err = runCmd(t, "plugin", "add", dir, "--kind", "provider", "--config", cfgDir, "--json")
	require.ErrorContains(t, err, "directory")
}

func TestPluginListReportsBrokenLink(t *testing.T) {
	pluginHome(t)
	cfgDir := t.TempDir()
	srcDir := t.TempDir()
	src := writePluginFile(t, srcDir, "plugins/providers/acme")
	_, err := runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.NoError(t, os.Remove(src))

	out, err := runCmd(t, "plugin", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	var listRep services.PluginListResult
	require.NoError(t, json.Unmarshal([]byte(out), &listRep))
	require.Len(t, listRep.Plugins, 1)
	require.True(t, listRep.Plugins[0].Broken)
}

// writeTargetPluginFile creates an executable target-plugin stub at
// <dir>/<rel> whose capability answer carries capability (a JSON object body
// without braces), so a test can vary what the plugin declares.
func writeTargetPluginFile(t *testing.T, dir, rel, id, capability string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	script := "#!/bin/sh\nread -r line\ncase \"$line\" in\n" +
		"  *capability*) echo '{" + capability + "}';;\n" +
		"  *) echo '{\"id\":\"" + id + "\",\"protocol_version\":2}';;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	return p
}

func TestPluginAddRegistersDeclaredTarget(t *testing.T) {
	pluginHome(t)
	cfgDir := t.TempDir()
	installDir := filepath.Join(t.TempDir(), "codefuse-skills")
	src := writeTargetPluginFile(t, t.TempDir(), "plugins/targets/codefuse", "codefuse",
		`"kinds":["skill"],"target_path":"`+installDir+`"`)

	out, err := runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.NoError(t, err)
	var addRep struct {
		Added services.PluginInfo `json:"added"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &addRep))
	require.NotNil(t, addRep.Added.Target, "a declared target_path must register a target")
	require.Equal(t, "codefuse", addRep.Added.Target.Name)
	require.Equal(t, installDir, addRep.Added.Target.Path)
	require.Empty(t, addRep.Added.Hint)

	// The target is persisted, so a later invocation sees it too.
	out, err = runCmd(t, "target", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, `"plugin:codefuse"`)

	// Re-linking with --force updates the stored entry to what the plugin now
	// declares instead of failing on the existing target.
	moved := filepath.Join(t.TempDir(), "moved")
	src2 := writeTargetPluginFile(t, t.TempDir(), "plugins/targets/codefuse", "codefuse",
		`"kinds":["skill"],"target_path":"`+moved+`"`)
	_, err = runCmd(t, "plugin", "add", src2, "--config", cfgDir, "--force", "--json")
	require.NoError(t, err)
	out, err = runCmd(t, "target", "list", "--config", cfgDir, "--json")
	require.NoError(t, err)
	require.Contains(t, out, moved)
}

func TestPluginAddHintsWhenTargetPluginDeclaresNoPath(t *testing.T) {
	pluginHome(t)
	cfgDir := t.TempDir()
	src := writeTargetPluginFile(t, t.TempDir(), "plugins/targets/nopath", "nopath", `"kinds":["skill","command"]`)

	out, err := runCmd(t, "plugin", "add", src, "--config", cfgDir, "--json")
	require.NoError(t, err)
	var addRep struct {
		Added services.PluginInfo `json:"added"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &addRep))
	require.Nil(t, addRep.Added.Target)
	require.Contains(t, addRep.Added.Hint, "skm target add --name nopath")
	require.Contains(t, addRep.Added.Hint, "--strategy skill=plugin:nopath")
	require.Contains(t, addRep.Added.Hint, "--strategy command=plugin:nopath")
}
