package tui

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

func (m *model) forkActions(e *common.Entry) []entryAction {
	if e.Kind != common.KindSkill || e.Status != common.StatusActive {
		return nil
	}
	st := e.SyncView
	if st == nil {
		return nil
	}

	run := func(mode string) func() {
		return func() {
			action := func() {
				m.submitJob(mode+" "+e.Name, func(ctx context.Context) (any, error) {
					var res *engines.UpdateResult
					var err error
					switch mode {
					case "fork":
						res, err = m.svc.Fork(ctx, e.Path, engines.SyncOptions{})
					case "replace fork":
						res, err = m.svc.Fork(ctx, e.Path, engines.SyncOptions{Replace: true})
					case "use upstream":
						res, err = m.svc.UseUpstream(ctx, e.Path, false)
					case "accept update":
						res, err = m.svc.AcceptUpdate(ctx, e.Path, false)
					case "cancel update":
						res, err = m.svc.CancelUpdate(ctx, e.Path, false)
					case "rollback update":
						res, err = m.svc.RollbackUpdate(ctx, e.Path, false)
					}
					if err != nil {
						return nil, err
					}
					path := res.ActivePath
					if path == "" {
						path = e.RepositoryPath
					}
					if path == "" {
						path = e.Path
					}
					return selectionJobResult{status: fmt.Sprintf("%s: %s", e.Name, res.Outcome), path: m.svc.Repo.RelPath(path)}, nil
				})
			}
			if mode == "fork" {
				action()
				return
			}
			prompt := mode + " for " + e.Name + "?"
			if mode == "replace fork" {
				prompt = "Delete the existing fork and recreate it from community content?"
			}
			m.confirm = &pages.Confirm{Prompt: prompt, OnYes: action}
		}
	}
	var out []entryAction
	if st.AttemptPath != "" {
		out = append(out, entryAction{Key: "menu", Label: "accept manually prepared update", Enabled: true, Run: run("accept update")}, entryAction{Key: "menu", Label: "cancel failed update", Enabled: true, Run: run("cancel update")})
	} else {
		if st.ForkState == "absent" {
			out = append(out, entryAction{Key: "menu", Label: "create fork", Enabled: true, Run: run("fork")})
		} else {
			if st.Mode == "fork" {
				out = append(out, entryAction{Key: "menu", Label: "use community version (retain fork)", Enabled: true, Run: run("use upstream")})
			}
			out = append(out, entryAction{Key: "menu", Label: "delete and recreate existing fork", Enabled: true, Run: run("replace fork")})
		}
		if st.RollbackAvailable {
			out = append(out, entryAction{Key: "menu", Label: "rollback last successful update", Enabled: true, Run: run("rollback update")})
		}
	}
	return out
}
