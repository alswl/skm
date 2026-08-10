package engines

import "github.com/alswl/skm/skm/pkg/common"

// DanglingInstall is a target-side installation that has no usable source.
// It is deliberately path-based: an orphan has no Entry to address it by.
type DanglingInstall struct {
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	TargetName string                 `json:"target"`
	Strategy   common.InstallStrategy `json:"-"`
}
