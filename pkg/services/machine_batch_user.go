package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/installer"
)

// machineUserRoot returns an existing canonical ancestor and the target path.
func (s *Services) machineUserRoot(name string) (string, string, error) {
	var target *common.InstallTarget
	for i := range s.Cfg.Targets {
		if s.Cfg.Targets[i].Name == name {
			if target != nil {
				return "", "", fmt.Errorf("duplicate target %q", name)
			}
			target = &s.Cfg.Targets[i]
		}
	}
	if target == nil {
		return "", "", fmt.Errorf("unknown target %q", name)
	}
	strategy, ok := target.EffectiveStrategy(common.KindSkill)
	if !target.AcceptsKind(common.KindSkill) || !ok || strategy != common.StrategySkillSymlink {
		return "", "", fmt.Errorf("target %q does not support direct skill links", name)
	}
	if !filepath.IsAbs(target.Path) {
		return "", "", fmt.Errorf("target %q path is not absolute", name)
	}
	path := filepath.Clean(target.Path)
	ancestor := path
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			info, err := os.Stat(resolved)
			if err != nil {
				return "", "", err
			}
			if !info.IsDir() {
				return "", "", fmt.Errorf("target ancestor %q is not a directory", ancestor)
			}
			rel, err := filepath.Rel(ancestor, path)
			if err != nil {
				return "", "", err
			}
			return resolved, filepath.Join(resolved, rel), nil
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", "", err
		}
		ancestor = parent
	}
}

func (s *Services) previewMachineBatchItems(request MachineBatchRequest, directories []MachineDirectoryInfo) ([]common.InstallationPlanItem, error) {
	if !request.User {
		return s.previewMachineDirectories(request.Skills, request.Targets, directories)
	}
	items := make([]common.InstallationPlanItem, 0)
	seen := make(map[string]int)
	claims := make(map[string]bool)
	for _, target := range request.Targets {
		_, targetPath, err := s.machineUserRoot(target)
		if err != nil {
			return nil, err
		}
		var rule string
		for _, t := range s.Cfg.Targets {
			if t.Name == target {
				rule = t.NameRule
				break
			}
		}
		for _, skill := range request.Skills {
			entry, err := s.ResolveEntry(skill)
			if err != nil || entry == nil || entry.Kind != common.KindSkill || entry.Status != common.StatusActive {
				return nil, fmt.Errorf("active skill %q not found", skill)
			}
			if rule != "" && !common.NameSatisfies(rule, entry.Name) {
				return nil, fmt.Errorf("skill %q does not satisfy target %q naming rule", skill, target)
			}
			ref := s.MachineSkillReference(entry)
			sources, err := installer.MachineSourceIndex([]installer.MachineSource{{Path: entry.Path, Skill: ref}})
			if err != nil {
				return nil, err
			}
			path := filepath.Join(targetPath, entry.Name)
			claim := path + "\x00" + ref
			if claims[claim] {
				continue
			}
			claims[claim] = true
			if index, ok := seen[path]; ok {
				previous := &items[index]
				if previous.Skill == ref {
					continue
				}
				previous.Status, previous.Error = "conflict", "multiple skills select this location"
				items = append(items, common.InstallationPlanItem{Target: target, Skill: ref, Path: path, Status: "conflict", Error: previous.Error})
				continue
			}
			seen[path] = len(items)
			item := common.InstallationPlanItem{Target: target, Skill: ref, Path: path}
			slot := installer.InspectMachineSlot(path, sources)
			switch slot.Status {
			case "missing":
				item.Status = "install"
			case "error":
				item.Status, item.Error = "error", slot.Error
			case "installed":
				item.Status = "installed"
			case "dangling":
				item.Status, item.Error = "conflict", "destination is a dangling link"
			default:
				item.Status, item.Error = "conflict", "destination is occupied by an external object"
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}
