package services

import (
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/managers/installer"
)

// installerFor rebuilds the Installer over targets, so an add/update/remove
// is immediately reflected in subsequent install/uninstall calls within the
// same Services instance.
func installerFor(targets []common.InstallTarget) *installer.Installer {
	return installer.New(targets)
}

// TargetInfo is one row of `target list` (contracts/cli-json.md).
type TargetInfo struct {
	Name       string                                      `json:"name"`
	Platform   string                                      `json:"platform"`
	Path       string                                      `json:"path"`
	Accepts    []common.EntryKind                          `json:"accepts"`
	Strategies map[common.EntryKind]common.InstallStrategy `json:"strategies"`
	Builtin    bool                                        `json:"builtin"`
	Valid      bool                                        `json:"valid"`
	PathState  string                                      `json:"path_state"`
	Error      *string                                     `json:"error"`
}

// TargetListResult is the CLI JSON report for `target list`.
type TargetListResult struct {
	ConfigDir string                 `json:"config_dir"`
	Targets   []TargetInfo           `json:"targets"`
	Invalid   []config.InvalidTarget `json:"invalid"`
}

// TargetList reports every loaded target plus any uninterpretable
// targets.json entry, each with its own reason (FR-016).
func (s *Services) TargetList() *TargetListResult {
	res := &TargetListResult{ConfigDir: s.Cfg.ConfigDir, Targets: []TargetInfo{}, Invalid: s.Cfg.InvalidTargets}
	for _, t := range s.Cfg.Targets {
		res.Targets = append(res.Targets, TargetInfo{
			Name: t.Name, Platform: t.Platform, Path: t.Path,
			Accepts: t.EffectiveAccepts(), Strategies: t.Strategies, Builtin: t.Builtin,
			Valid: true, PathState: config.PathState(t.Path),
		})
	}
	if res.Invalid == nil {
		res.Invalid = []config.InvalidTarget{}
	}
	return res
}

// TargetAdd validates and persists a new target (FR-010).
func (s *Services) TargetAdd(t common.InstallTarget) (common.InstallTarget, error) {
	added, err := config.AddTarget(s.Cfg.ConfigDir, t)
	if err != nil {
		return common.InstallTarget{}, common.WithExitCode(err, common.ExitError)
	}
	s.Cfg.Targets = append(s.Cfg.Targets, added)
	s.Installer = installerFor(s.Cfg.Targets)
	return added, nil
}

// TargetUpdate applies apply to the named target, re-validates, and persists
// it (FR-010).
func (s *Services) TargetUpdate(name string, apply func(*common.InstallTarget)) (common.InstallTarget, error) {
	updated, err := config.UpdateTarget(s.Cfg.ConfigDir, name, apply)
	if err != nil {
		return common.InstallTarget{}, common.WithExitCode(err, common.ExitError)
	}
	for i, t := range s.Cfg.Targets {
		if t.Name == name {
			s.Cfg.Targets[i] = updated
		}
	}
	s.Installer = installerFor(s.Cfg.Targets)
	return updated, nil
}

// TargetRemove deletes the named target (FR-010). Assets already installed
// through it are left untouched (FR-018).
func (s *Services) TargetRemove(name string) error {
	if err := config.RemoveTarget(s.Cfg.ConfigDir, name); err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	out := make([]common.InstallTarget, 0, len(s.Cfg.Targets))
	for _, t := range s.Cfg.Targets {
		if t.Name != name {
			out = append(out, t)
		}
	}
	s.Cfg.Targets = out
	s.Installer = installerFor(s.Cfg.Targets)
	return nil
}

// TargetValidateEntry is one result of `target validate`.
type TargetValidateEntry struct {
	Name      string  `json:"name"`
	OK        bool    `json:"ok"`
	PathState string  `json:"path_state"`
	Error     *string `json:"error"`
}

// TargetValidateResult is the CLI JSON report for `target validate`.
type TargetValidateResult struct {
	Results []TargetValidateEntry `json:"results"`
	Success bool                  `json:"success"`
}

// TargetValidate reports pass/fail-with-reason for every target, or just
// name when non-empty (SC-009). It re-validates the schema and reports path
// usability without performing an install (FR-014).
func (s *Services) TargetValidate(name string) *TargetValidateResult {
	res := &TargetValidateResult{Success: true}
	for _, t := range s.Cfg.Targets {
		if name != "" && t.Name != name {
			continue
		}
		entry := TargetValidateEntry{Name: t.Name, PathState: config.PathState(t.Path), OK: true}
		if reason := config.ValidateTarget(t); reason != "" {
			entry.OK = false
			entry.Error = &reason
		}
		if !entry.OK {
			res.Success = false
		}
		res.Results = append(res.Results, entry)
	}
	if name != "" && len(res.Results) == 0 {
		reason := fmt.Sprintf("target %q not found", name)
		res.Results = append(res.Results, TargetValidateEntry{Name: name, OK: false, Error: &reason})
		res.Success = false
	}
	return res
}
