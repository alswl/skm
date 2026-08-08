package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

// TestSingleFileCommandFullLifecycle exercises every core write operation —
// install via both command strategies, uninstall, archive, unarchive,
// delete — against a self-build single-file command (entry.Path is the .md
// marker file itself, not a directory, per Entry.MarkerPath). This is the
// exact shape that crashed command-adapter install with "list resources for
// %q: ...: not a directory" (fixed in command_adapter.go's entryResources).
// This test drives the same Services entry points the CLI and TUI use, to
// catch any other core-chain step that assumes a directory-shaped entry.
func TestSingleFileCommandFullLifecycle(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "commands", "self-build"), 0o755))
	markerPath := filepath.Join(root, "commands", "self-build", "flatcmd.md")
	require.NoError(t, os.WriteFile(markerPath,
		[]byte("---\nname: flatcmd\ndescription: a hand-authored command\n---\nbody\n"), 0o644))

	markerTarget := filepath.Join(root, "targets", "marker")
	adapterTarget := filepath.Join(root, "targets", "adapter")
	require.NoError(t, os.MkdirAll(markerTarget, 0o755))
	require.NoError(t, os.MkdirAll(adapterTarget, 0o755))
	targets := []common.InstallTarget{
		{Name: "marker", Path: markerTarget, Kind: common.KindCommand}, // command-marker strategy
		{Name: "adapter", Path: adapterTarget, Kind: common.KindSkill}, // command-adapter strategy
	}
	svc, err := New(newCfg(root, targets), common.NewLogger(false))
	require.NoError(t, err)

	entry := svc.FindEntry("flatcmd")
	require.NotNil(t, entry)
	require.Equal(t, markerPath, entry.Path, "self-build entry.Path is the flat .md file itself, not a directory")
	require.Nil(t, entry.Origin, "self-build entries are hand-authored, never fetched, so they carry no origin")
	require.Equal(t, common.StatusActive, entry.Status)

	ctx := context.Background()
	adapterDir := filepath.Join(adapterTarget, "flatcmd")

	// Install into both strategies at once.
	res, err := svc.Install(ctx, "flatcmd", InstallOptions{})
	require.NoError(t, err, "install must not crash on a single-file command (command-adapter regression)")
	require.True(t, res.Success)
	require.Len(t, res.Results, 2)
	require.True(t, dal.IsSymlink(filepath.Join(markerTarget, "flatcmd.md")), "command-marker strategy links the file directly")
	require.True(t, dal.IsDir(adapterDir), "command-adapter strategy wraps the file in an adapter dir")
	require.FileExists(t, filepath.Join(adapterDir, dal.AdapterMarker))
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, targets[0]))
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, targets[1]))

	// Uninstall from both: never touches the real repo file, only the
	// target-side managed links/adapters.
	_, err = svc.Uninstall(ctx, "flatcmd", InstallOptions{})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(markerTarget, "flatcmd.md"))
	require.NoDirExists(t, adapterDir)
	require.FileExists(t, markerPath, "uninstall never removes the repo entry itself")

	// Re-install so archive's uninstall-first step has something real to undo.
	_, err = svc.Install(ctx, "flatcmd", InstallOptions{})
	require.NoError(t, err)

	// Archive: the TUI uninstalls first (actions_lifecycle.go), then archives.
	_, err = svc.Uninstall(ctx, "flatcmd", InstallOptions{})
	require.NoError(t, err)
	_, err = svc.Archive(ctx, "flatcmd", LifecycleOptions{})
	require.NoError(t, err, "archiving a single-file command must not crash")
	require.NoDirExists(t, adapterDir, "archive uninstalled the adapter first")

	archived := svc.FindEntry("flatcmd")
	require.NotNil(t, archived, "Scan() includes archived entries")
	require.Equal(t, common.StatusArchived, archived.Status)
	require.FileExists(t, archived.Path, "the flat .md file survives the move to archived/ as a file, not a directory")

	// Unarchive back to active.
	_, err = svc.Unarchive(ctx, "flatcmd", LifecycleOptions{})
	require.NoError(t, err, "unarchiving a single-file command must not crash")
	restored := svc.FindEntry("flatcmd")
	require.NotNil(t, restored)
	require.Equal(t, common.StatusActive, restored.Status)
	require.Equal(t, markerPath, restored.Path, "unarchive restores the exact original path")

	// Delete (requires Force, matching the TUI's confirm-then-Force:true
	// pattern in actions_lifecycle.go's deleteSelected).
	_, err = svc.Delete(ctx, "flatcmd", LifecycleOptions{Force: true})
	require.NoError(t, err, "deleting a single-file command must not crash")
	require.Nil(t, svc.FindEntry("flatcmd"))
	require.NoFileExists(t, markerPath)
}
