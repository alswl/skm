package tui

import (
	"context"
	"fmt"
)

// discoverExternal lists external unmanaged skills in the install targets and
// opens a multi-select modal to adopt them into the repository (replacing each
// with a managed symlink) or delete them after confirmation (key "o",
// FR-038).
func (m *model) discoverExternal() {
	res := m.svc.Discover("")
	if len(res.Found) == 0 {
		m.status = "discover: no external unmanaged skills found"
		return
	}
	items := make([]pickerItem, len(res.Found))
	for i, f := range res.Found {
		items[i] = pickerItem{label: fmt.Sprintf("%s  (%s)", f.Name, f.Path), value: f.Path}
	}
	m.picker = &picker{
		title:     "discover external skills",
		hint:      "[space] toggle  [enter] adopt  [d] delete  [esc/q] cancel",
		items:     items,
		onConfirm: m.adoptExternal,
		onDelete:  m.confirmDeleteExternal,
	}
}

// adoptExternal adopts each selected external skill in the background: import
// into the repo and replace the external directory with a managed symlink.
func (m *model) adoptExternal(sel []pickerItem) {
	if len(sel) == 0 {
		m.status = "adopt: nothing selected"
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
func (m *model) confirmDeleteExternal(sel []pickerItem) {
	if len(sel) == 0 {
		m.status = "delete: nothing selected"
		return
	}
	paths := pickerValues(sel)
	m.confirm = &confirm{
		prompt: fmt.Sprintf("Delete %d external skill director(y/ies)? This removes real files.", len(paths)),
		onYes: func() {
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

// pickerValues extracts the opaque values from picker items.
func pickerValues(sel []pickerItem) []string {
	out := make([]string, len(sel))
	for i, it := range sel {
		out[i] = it.value
	}
	return out
}
