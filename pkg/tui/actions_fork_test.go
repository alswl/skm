package tui

import (
	"context"
	"testing"

	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/services"
	"github.com/stretchr/testify/require"
)

func TestForkActionsUseSharedServiceAndConfirmReplacement(t *testing.T) {
	m := newTestModel(t)
	src := t.TempDir()
	writeFileT(t, src, "SKILL.md", "---\nname: review\ndescription: review\n---\nbody\n")
	_, err := m.svc.Import(context.Background(), src, services.ImportOptions{})
	require.NoError(t, err)
	entry := m.svc.FindEntry("review")
	require.NotNil(t, entry)
	actions := m.forkActions(entry)
	require.Len(t, actions, 1)
	require.Equal(t, "create fork", actions[0].Label)
	actions[0].Run()
	drainJob(t, &m)
	entry = m.svc.FindEntry("review")
	require.Equal(t, "fork", entry.SyncView.Mode)
	actions = m.forkActions(entry)
	require.Len(t, actions, 2)
	actions[1].Run()
	require.NotNil(t, m.confirm)
	require.Contains(t, m.confirm.Prompt, "Delete")
	// The confirmation itself must not remove customized content.
	require.DirExists(t, entry.Path)
	_, err = m.svc.UseUpstream(context.Background(), "review", false)
	require.NoError(t, err)
	entry = m.svc.FindEntry("review")
	actions = m.forkActions(entry)
	require.Len(t, actions, 1)
	require.Contains(t, actions[0].Label, "recreate")
	_, err = m.svc.Fork(context.Background(), "review", engines.SyncOptions{})
	require.ErrorContains(t, err, "existing_fork")
}
