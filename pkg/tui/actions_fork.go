package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// forkActions are the fork rows of the actions menu. They call the same
// Services.Fork as `skm fork`; replacing or discarding deletes the user's
// copy, so those two confirm first.
func (m *model) forkActions(e *common.Entry) []entryAction {
	if e.Kind != common.KindSkill || e.Status != common.StatusActive {
		return nil
	}
	run := func(label string, opts services.ForkOptions, prompt string) func() {
		do := func() {
			m.submitJob(label+" "+e.Name, func(ctx context.Context) (any, error) {
				res, err := m.svc.Fork(ctx, e.Path, opts)
				if err != nil {
					return nil, err
				}
				return selectionJobResult{status: fmt.Sprintf("%s: %s", e.Name, res.Action), path: m.svc.Repo.RelPath(res.ActivePath)}, nil
			})
		}
		if prompt == "" {
			return do
		}
		return func() { m.confirm = &pages.Confirm{Prompt: prompt, OnYes: do} }
	}
	if e.UpstreamPath == "" {
		return []entryAction{{Key: "menu", Label: "create fork", Enabled: true, Run: run("fork", services.ForkOptions{}, "")}}
	}
	return []entryAction{
		{Key: "menu", Label: "replace fork (copy upstream again)", Enabled: true, Run: run("replace fork", services.ForkOptions{Replace: true},
			fmt.Sprintf("Delete your fork of %s and copy the upstream again?", e.Name))},
		{Key: "menu", Label: "discard fork (back to upstream)", Enabled: true, Run: run("discard fork", services.ForkOptions{Discard: true},
			fmt.Sprintf("Delete your fork of %s and go back to the upstream?", e.Name))},
	}
}

// refuseForked reports whether entry has a fork, telling the user to discard it
// first. Archive and delete move or remove the entry's directory, which would
// leave the upstream behind.
func (m *model) refuseForked(entry *common.Entry, action string) bool {
	if entry.UpstreamPath == "" {
		return false
	}
	m.setStatus(fmt.Sprintf("%s: %s has a fork; discard the fork first (actions menu)", action, entry.Name))
	return true
}
