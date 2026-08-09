package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/services"
	pages "github.com/alswl/skm/skm/pkg/tui/widgets"
)

// discoverExternal lists external unmanaged skills in the install targets and
// opens a multi-select modal to adopt them into the repository (replacing each
// with a managed symlink) or delete them after confirmation (key "o",
// FR-038). It also surfaces any provider plugin that failed to load during
// discovery (providers.Registry.LoadFailures(), via Services.ProviderList()):
// previously that was visible only through the CLI's `provider list/validate`
// and never shown in the TUI (FR-003; R3).
func (m *model) discoverExternal() {
	failureMsg := providerLoadFailureSummary(m.svc)
	res := m.svc.Discover("")
	switch {
	case len(res.Found) == 0 && failureMsg != "":
		m.setStatus(failureMsg)
		return
	case len(res.Found) == 0:
		m.setStatus("discover: no external unmanaged skills found")
		return
	case failureMsg != "":
		m.setStatus(failureMsg)
	}
	items := make([]pages.PickerItem, len(res.Found))
	for i, f := range res.Found {
		items[i] = pages.PickerItem{Label: fmt.Sprintf("%s  (%s)", f.Name, f.Path), Value: f.Path}
	}
	m.picker = &pages.Picker{
		Title:     "discover external skills",
		Hint:      "[space] toggle  [enter] adopt  [d] delete  [esc/q] cancel",
		Items:     items,
		OnConfirm: m.adoptExternal,
		OnDelete:  m.confirmDeleteExternal,
	}
}

// adoptExternal adopts each selected external skill in the background: import
// into the repo and replace the external directory with a managed symlink.
func (m *model) adoptExternal(sel []pages.PickerItem) {
	if len(sel) == 0 {
		m.setStatus("adopt: nothing selected")
		return
	}
	paths := pickerValues(sel)
	m.submitJob(fmt.Sprintf("adopt %d skill(s)", len(paths)), func(ctx context.Context) (any, error) {
		var names []string
		for _, p := range paths {
			res, err := m.svc.AdoptExternal(ctx, p, false)
			if err != nil {
				return nil, err
			}
			names = append(names, res.Name)
		}
		return fmt.Sprintf("adopted %d skill(s): %v", len(names), names), nil
	})
}

// confirmDeleteExternal asks for confirmation, then deletes the selected
// external skill directories in the background (FR-038, FR-040).
func (m *model) confirmDeleteExternal(sel []pages.PickerItem) {
	if len(sel) == 0 {
		m.setStatus("delete: nothing selected")
		return
	}
	paths := pickerValues(sel)
	m.confirm = &pages.Confirm{
		Prompt: fmt.Sprintf("Delete %d external skill director(y/ies)? This removes real files.", len(paths)),
		OnYes: func() {
			m.submitJob(fmt.Sprintf("delete %d external", len(paths)), func(ctx context.Context) (any, error) {
				for _, p := range paths {
					if err := m.svc.DeleteExternal(p); err != nil {
						return nil, err
					}
				}
				return fmt.Sprintf("deleted %d external skill(s)", len(paths)), nil
			})
		},
	}
}

// providerLoadFailureSummary reports any provider plugins that failed to load
// during discovery (populated at startup, before discoverExternal ever runs),
// so a user running discover — the natural place to look for provider health
// — sees the specific reason instead of only the CLI's `provider
// list`/`validate`.
func providerLoadFailureSummary(svc *services.Services) string {
	var failures []string
	for _, p := range svc.ProviderList().Providers {
		if !p.Loaded {
			failures = append(failures, fmt.Sprintf("%s (%s): %s", p.ID, p.Path, p.Error.Message))
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return fmt.Sprintf("discover: %d provider plugin(s) failed to load — %s", len(failures), strings.Join(failures, "; "))
}

// pickerValues extracts the opaque values from picker items.
func pickerValues(sel []pages.PickerItem) []string {
	out := make([]string, len(sel))
	for i, it := range sel {
		out[i] = it.Value
	}
	return out
}
