package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestPluginAddRegistersTargetAndHonorsForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfgDir := t.TempDir()
	cfg, err := config.LoadForDeploy(cfgDir)
	require.NoError(t, err)
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "acme-target")
	writeRegistrationPlugin(t, path, "acme-target", filepath.Join(t.TempDir(), "first"))
	info, err := svc.PluginAdd(path, "target", "", false)
	require.NoError(t, err)
	require.NotNil(t, info.Target)
	require.Equal(t, "acme-target", info.Target.Name)

	// A pre-existing target is preserved until force is explicit.
	second := filepath.Join(t.TempDir(), "second")
	path2 := filepath.Join(t.TempDir(), "replacement")
	writeRegistrationPlugin(t, path2, "acme-target", second)
	info, err = svc.PluginAdd(path2, "target", "acme-target", true)
	require.NoError(t, err)
	require.NotNil(t, info.Target)
	require.Equal(t, second, info.Target.Path)
}

func TestPluginAddWithoutTargetMetadataKeepsLinkAndHint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadForDeploy(t.TempDir())
	require.NoError(t, err)
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "no-path")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nread -r line\ncase \"$line\" in *capability*) echo '{\"id\":\"no-path\",\"kinds\":[\"skill\"]}' ;; *) echo '{\"id\":\"no-path\",\"protocol_version\":2}' ;; esac\n"), 0o755))
	info, err := svc.PluginAdd(path, "target", "", false)
	require.NoError(t, err)
	require.Nil(t, info.Target)
	require.Contains(t, info.Hint, "skm target add")
}

func writeRegistrationPlugin(t *testing.T, path, id, target string) {
	t.Helper()
	script := "#!/bin/sh\nread -r line\ncase \"$line\" in *capability*) echo '{\"id\":\"" + id + "\",\"kinds\":[\"skill\"],\"target_path\":\"" + target + "\"}' ;; *) echo '{\"id\":\"" + id + "\",\"protocol_version\":2}' ;; esac\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}
