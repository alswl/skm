package tui

import "github.com/alswl/skm/skm/pkg/jobs"

// Custom tea.Msg types live here, one home each (go-tui-guides.md: "Communicate
// results with typed messages").
//
// jobDoneMsg carries a completed background job's result to the UI (FR-010).
type jobDoneMsg struct {
	Result jobs.Result
}
