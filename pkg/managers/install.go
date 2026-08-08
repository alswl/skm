package managers

import (
	"context"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// InstallOptions controls an install/uninstall action.
type InstallOptions struct {
	// Targets names to act on; empty means all kind-matching targets.
	Targets []string
	Force   bool
	DryRun  bool
}

// InstallResult is the CLI JSON report for install/uninstall
// (contract/cli-json.md).
type InstallResult struct {
	Action  string                 `json:"action"`
	Name    string                 `json:"name"`
	DryRun  bool                   `json:"dry_run"`
	Results []common.InstallReport `json:"results"`
	Success bool                   `json:"success"`
}

// Install installs entry into the selected targets (FR-014..FR-018).
func (s *Services) Install(ctx context.Context, name string, opts InstallOptions) (*InstallResult, error) {
	return s.runInstall(ctx, "install", name, opts)
}

// Uninstall removes only managed installs of entry (FR-017).
func (s *Services) Uninstall(ctx context.Context, name string, opts InstallOptions) (*InstallResult, error) {
	return s.runInstall(ctx, "uninstall", name, opts)
}

func (s *Services) runInstall(ctx context.Context, action, name string, opts InstallOptions) (*InstallResult, error) {
	entry := s.FindEntry(name)
	if entry == nil {
		return nil, common.WithExitCode(fmt.Errorf("%s: entry %q not found", action, name), common.ExitObject)
	}
	if entry.Status != common.StatusActive {
		return nil, common.WithExitCode(fmt.Errorf("%s: entry %q is %s; only active entries can be %sd", action, name, entry.Status, action), common.ExitObject)
	}
	targets, err := s.selectTargets(entry, opts.Targets)
	if err != nil {
		return nil, err
	}
	result := &InstallResult{Action: action, Name: name, DryRun: opts.DryRun, Results: []common.InstallReport{}}

	if opts.DryRun {
		for _, t := range targets {
			report := common.InstallReport{Target: t.Name, Status: s.Installer.State(entry, t)}
			report.Changed = report.Status != desiredInstallState(action)
			report.Status = desiredInstallState(action)
			result.Results = append(result.Results, report)
		}
		result.Success = true
		return result, nil
	}

	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	tx := &dal.FileTransaction{}
	for _, t := range targets {
		var changed bool
		var err error
		if action == "install" {
			changed, err = s.Installer.Install(tx, entry, t, opts.Force)
		} else {
			changed, err = s.Installer.Uninstall(tx, entry, t)
		}
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		result.Results = append(result.Results, common.InstallReport{
			Target:  t.Name,
			Status:  desiredInstallState(action),
			Changed: changed,
		})
	}
	tx.Commit()
	result.Success = true
	return result, nil
}

// selectTargets resolves explicit target names (empty = all kind-matching).
// Unknown target names or targets that cannot receive the entry are argument
// errors (exit 2 per exit-codes.md).
func (s *Services) selectTargets(entry *common.Entry, names []string) ([]common.InstallTarget, error) {
	if len(names) == 0 {
		return s.Installer.Targets(entry), nil
	}
	var out []common.InstallTarget
	for _, n := range names {
		t, ok := s.Installer.TargetByName(n)
		if !ok {
			return nil, common.WithExitCode(fmt.Errorf("unknown target %q", n), common.ExitError)
		}
		if !s.Installer.Matches(entry, t) {
			return nil, common.WithExitCode(fmt.Errorf("target %q does not accept a %s entry", n, entry.Kind), common.ExitError)
		}
		out = append(out, t)
	}
	return out, nil
}

func desiredInstallState(action string) common.InstallState {
	if action == "install" {
		return common.InstallInstalled
	}
	return common.InstallAbsent
}
