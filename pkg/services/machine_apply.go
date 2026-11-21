package services

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MachineBatchResult reports independent installation outcomes.
type MachineBatchResult struct {
	Items []common.InstallationPlanItem `json:"items"`
}

// ApplyMachineBatch re-plans under the library lock, merges the confirmed
// declarations, and independently attempts each currently safe link.
func (s *Services) ApplyMachineBatch(ctx context.Context, selected MachineBatchPlan) (*MachineBatchResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	current, err := s.PlanMachineBatch(selected.Request)
	if err != nil {
		return nil, err
	}
	if selected.Action != "add" || selected.Version != 1 || current.Digest != selected.Digest {
		return nil, fmt.Errorf("installation plan is stale; create a new preview")
	}
	if len(current.Declarations) == 0 {
		return s.executeMachineBatchItems(current, nil), nil
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return nil, err
	}
	for _, declaration := range current.Declarations {
		if !slices.Contains(doc.Installs, declaration.Install) {
			doc.Installs = append(doc.Installs, declaration.Install)
		}
	}
	if err := config.WriteYAMLAtomic(manifest, doc, 0o644); err != nil {
		return nil, err
	}
	return s.executeMachineBatchItems(current, nil), nil
}

func (s *Services) executeMachineBatchItems(current *MachineBatchPlan, seen map[string]bool) *MachineBatchResult {
	result := &MachineBatchResult{Items: append([]common.InstallationPlanItem(nil), current.Items...)}
	for i := range result.Items {
		item := &result.Items[i]
		if seen != nil && item.Path != "" {
			if seen[item.Path] {
				item.Status = "duplicate"
				continue
			}
			seen[item.Path] = true
		}
		if item.Status != "install" && (item.Status != "unignored" || !current.Request.AllowUnignored) {
			continue
		}
		// Recheck immediately before creation; CreateMachineLink enforces the
		// boundary and destination non-overwrite even if parents change.
		one := current.Request
		one.Skills, one.Targets = []string{item.Skill}, []string{item.Target}
		var selectedDirectories []MachineDirectoryInfo
		for _, directory := range current.Directories {
			if directory.ID == item.Directory {
				selectedDirectories = append(selectedDirectories, directory)
				break
			}
		}
		fresh, err := s.previewMachineBatchItems(one, selectedDirectories)
		if err != nil {
			item.Status = "error"
			item.Error = err.Error()
			continue
		}
		valid := false
		for _, candidate := range fresh {
			if candidate.Directory == item.Directory && candidate.Path == item.Path && (candidate.Status == "install" || (candidate.Status == "unignored" && current.Request.AllowUnignored)) {
				valid = true
			}
		}
		if !valid {
			item.Status = "changed"
			continue
		}
		var root string
		if current.Request.User {
			root, _, err = s.machineUserRoot(item.Target)
			if err != nil {
				item.Status, item.Error = "error", err.Error()
				continue
			}
		}
		for _, directory := range current.Directories {
			if directory.ID == item.Directory {
				root = directory.Path
				break
			}
		}
		rel, err := filepath.Rel(root, item.Path)
		entry, resolveErr := s.ResolveEntry(item.Skill)
		if err == nil && resolveErr == nil && entry == nil {
			resolveErr = fmt.Errorf("skill %q no longer exists", item.Skill)
		}
		if err == nil && resolveErr == nil {
			var expected *machineBatchSource
			for i := range current.Sources {
				if current.Sources[i].Skill == item.Skill {
					expected = &current.Sources[i]
					break
				}
			}
			if expected == nil || entry.Path != expected.Path {
				resolveErr = fmt.Errorf("skill %q source changed after preview", item.Skill)
			} else {
				hash, hashErr := dal.DirHash(entry.Path, nil)
				if hashErr != nil {
					resolveErr = fmt.Errorf("inspect skill %q: %w", item.Skill, hashErr)
				} else if hash != expected.Content {
					resolveErr = fmt.Errorf("skill %q content changed after preview", item.Skill)
				}
			}
		}
		if err == nil && resolveErr == nil {
			err = dal.CreateMachineLink(root, rel, entry.Path)
		}
		if err != nil || resolveErr != nil {
			item.Status = "error"
			if err != nil {
				item.Error = err.Error()
			} else {
				item.Error = resolveErr.Error()
			}
			continue
		}
		item.Status = "installed"
	}
	return result
}
