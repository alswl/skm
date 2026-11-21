package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/installer"
)

// PreviewMachineTags expands selected tags and checks current disk state.
func (s *Services) PreviewMachineTags(skills, targets, tags []string) ([]common.InstallationPlanItem, error) {
	directories, err := s.SelectMachineDirectories(tags)
	if err != nil {
		return nil, err
	}
	return s.previewMachineDirectories(skills, targets, directories)
}

func (s *Services) previewMachineDirectories(skills, targets []string, directories []MachineDirectoryInfo) ([]common.InstallationPlanItem, error) {
	for _, target := range targets {
		if target != "codex" && target != "claude" {
			return nil, fmt.Errorf("unsupported directory target %q", target)
		}
	}
	result := make([]common.InstallationPlanItem, 0)
	seen := make(map[string]int)
	claims := make(map[string]bool)
	for _, skill := range skills {
		entry, err := s.ResolveEntry(skill)
		if err != nil {
			return nil, err
		}
		if entry == nil || entry.Kind != common.KindSkill || entry.Status != common.StatusActive {
			return nil, fmt.Errorf("active skill %q not found", skill)
		}
		ref := s.MachineSkillReference(entry)
		sources, err := installer.MachineSourceIndex([]installer.MachineSource{{Path: entry.Path, Skill: ref}})
		if err != nil {
			return nil, err
		}
		for _, directory := range directories {
			for _, target := range targets {
				var targetPath string
				for _, resolved := range installer.MachineDirectoryTargets(directory.Path) {
					if resolved.Name == target {
						targetPath = resolved.Path
						break
					}
				}
				item := common.InstallationPlanItem{Directory: directory.ID, Target: target, Skill: ref}
				if directory.Path == "" {
					item.Status = "missing_path"
					result = append(result, item)
					continue
				}
				item.Path = filepath.Join(targetPath, entry.Name)
				claim := item.Path + "\x00" + ref
				if claims[claim] {
					continue
				}
				claims[claim] = true
				if index, ok := seen[item.Path]; ok {
					previous := &result[index]
					if previous.Skill == ref {
						continue
					}
					previous.Status = "conflict"
					previous.Error = "multiple skills select this location"
					item.Status, item.Error = "conflict", previous.Error
					result = append(result, item)
					continue
				}
				seen[item.Path] = len(result)
				if err := machineSafeParents(directory.Path, filepath.Dir(item.Path)); err != nil {
					item.Status = "conflict"
					item.Error = err.Error()
					result = append(result, item)
					continue
				}
				gitStatus, err := engines.MachineGitSlotStatus(directory.Path, item.Path)
				if err != nil {
					item.Status = "error"
					item.Error = err.Error()
					result = append(result, item)
					continue
				}
				if gitStatus == "conflict" {
					item.Status = "conflict"
					item.Error = "destination is tracked by Git"
					result = append(result, item)
					continue
				}
				slot := installer.InspectMachineSlot(item.Path, sources)
				switch slot.Status {
				case "missing":
					item.Status = gitStatus
				case "error":
					item.Status, item.Error = "error", slot.Error
				case "installed":
					item.Status = "installed"
				case "dangling":
					item.Status, item.Error = "conflict", "destination is a dangling link"
				default:
					item.Status, item.Error = "conflict", "destination is occupied by an external object"
				}
				result = append(result, item)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Directory != result[j].Directory {
			return result[i].Directory < result[j].Directory
		}
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		return result[i].Skill < result[j].Skill
	})
	return result, nil
}

func machineSafeParents(root, parent string) error {
	rel, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("destination outside directory")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe parent %q", current)
		}
	}
	return nil
}
