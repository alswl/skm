package services

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/installer"
)

// ScanMachineDirectory reads the two supported project skill directories.
func (s *Services) ScanMachineDirectory(id string) ([]common.InstallationObservation, error) {
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	var root string
	found := false
	for _, directory := range directories {
		if directory.ID == id {
			found = true
			root = directory.Path
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown directory %q", id)
	}
	if root == "" {
		return nil, fmt.Errorf("directory %q has no local path", id)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("directory %q: %w", id, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("directory %q path is not a directory", id)
	}
	return s.scanMachinePath(id, root, true)
}

// ScanMachinePath scans an unregistered Git worktree root.
func (s *Services) ScanMachinePath(path string) ([]common.InstallationObservation, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	output, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect Git directory %q: %w", path, err)
	}
	gitRoot, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil || gitRoot != root {
		return nil, fmt.Errorf("directory %q is not a Git root", path)
	}
	return s.scanMachinePath("", root, false)
}

func (s *Services) scanMachinePath(id, root string, desired bool) ([]common.InstallationObservation, error) {
	targets := installer.MachineDirectoryTargets(root)
	return s.scanMachineTargets(id, false, targets, desired)
}

// ScanMachineAll scans user targets and every registered directory.
func (s *Services) ScanMachineAll() ([]common.InstallationObservation, error) {
	result, err := s.ScanMachineUser()
	if err != nil {
		return nil, err
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		if directory.Path == "" {
			result = append(result, common.InstallationObservation{Scope: "directory", Directory: directory.ID, Status: "error", Error: "directory has no local path"})
			continue
		}
		items, err := s.ScanMachineDirectory(directory.ID)
		if err != nil {
			result = append(result, common.InstallationObservation{Scope: "directory", Directory: directory.ID, Path: directory.Path, Status: "error", Error: err.Error()})
			continue
		}
		result = append(result, items...)
	}
	return result, nil
}

// FilterMachineSkill selects observations for one library skill while keeping
// diagnostics that could make the scan incomplete.
func (s *Services) FilterMachineSkill(items []common.InstallationObservation, skill string) ([]common.InstallationObservation, error) {
	entry, err := s.ResolveEntry(skill)
	if err != nil {
		return nil, err
	}
	if entry == nil || entry.Kind != common.KindSkill {
		return nil, fmt.Errorf("skill %q not found", skill)
	}
	ref := s.MachineSkillReference(entry)
	filtered := make([]common.InstallationObservation, 0, len(items))
	for _, item := range items {
		if item.Skill == ref || slices.Contains(item.ExpectedSkills, ref) || item.Status == "error" || item.Status == "recovery" {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// ScanMachineUser reads configured user-level skill targets.
func (s *Services) ScanMachineUser() ([]common.InstallationObservation, error) {
	return s.scanMachineUser(true)
}

func (s *Services) scanMachineUser(desired bool) ([]common.InstallationObservation, error) {
	targets := installer.MachineUserTargets(s.Cfg.Targets)
	return s.scanMachineTargets("", true, targets, desired)
}

func (s *Services) scanMachineTargets(id string, user bool, targets []installer.MachineTarget, desired bool) ([]common.InstallationObservation, error) {
	scope := "path"
	if user {
		scope = "user"
	} else if id != "" {
		scope = "directory"
	}
	var sources, upstreamSources []installer.MachineSource
	for _, entry := range s.Scan() {
		if entry.Kind == common.KindSkill && entry.Status == common.StatusActive {
			ref := s.MachineSkillReference(entry)
			sources = append(sources, installer.MachineSource{Path: entry.Path, Skill: ref})
			if entry.UpstreamPath != "" {
				upstreamSources = append(upstreamSources, installer.MachineSource{Path: entry.UpstreamPath, Skill: ref})
			}
		}
	}
	library, sourceDiagnostics := installer.InspectMachineSources(sources)
	upstream, upstreamDiagnostics := installer.InspectMachineSources(upstreamSources)
	sourceDiagnostics = append(sourceDiagnostics, upstreamDiagnostics...)
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return nil, err
	}
	result := make([]common.InstallationObservation, 0)
	for _, diagnostic := range sourceDiagnostics {
		result = append(result, common.InstallationObservation{Scope: scope, Directory: id, Path: diagnostic.Path, Skill: diagnostic.Skill, Status: "error", Error: diagnostic.Error})
	}
	for _, target := range targets {
		entries, err := os.ReadDir(target.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			result = append(result, common.InstallationObservation{Scope: scope, Directory: id, Target: target.Name, Path: target.Path, Status: "error", Error: err.Error()})
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(target.Path, entry.Name())
			if entry.IsDir() && strings.HasPrefix(entry.Name(), ".skm-seized-") {
				suffix := strings.TrimPrefix(entry.Name(), ".skm-seized-")
				if len(suffix) == 32 {
					if _, err := hex.DecodeString(suffix); err == nil {
						retained := filepath.Join(path, "object")
						if _, err := os.Lstat(retained); os.IsNotExist(err) {
							retained = path
						} else if err != nil {
							result = append(result, common.InstallationObservation{Scope: scope, Directory: id, Target: target.Name, Path: path, Status: "error", Error: err.Error()})
							continue
						}
						result = append(result, common.InstallationObservation{Scope: scope, Directory: id, Target: target.Name, Path: retained, Status: "recovery", Error: "interrupted link removal; inspect retained object"})
						continue
					}
				}
			}
			slot := installer.InspectMachineSlot(path, library)
			if slot.Status == "external" {
				old := installer.InspectMachineSlot(path, upstream)
				if old.Status == "installed" {
					slot.Status, slot.Skill = "source_changed", old.Skill
				}
			}
			if slot.Status == "missing" {
				slot.Status, slot.Error = "error", "slot disappeared during scan"
			}
			item := common.InstallationObservation{Scope: scope, Directory: id, Target: target.Name, Path: path, Status: slot.Status, Skill: slot.Skill, Error: slot.Error}
			if item.Skill != "" {
				if user {
					item.Declared = machineUserCovered(doc, item.Target, item.Skill)
				} else if id != "" {
					item.Declared = machineDirectoryCovered(doc, id, item.Target, item.Skill)
				}
			}
			result = append(result, item)
		}
	}
	if desired && (user || id != "") {
		request := MachineRestoreRequest{User: user}
		if id != "" {
			request.Directories = []string{id}
		}
		result, err = s.appendMachineDesired(result, request)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// appendMachineDesired adds currently absent declared locations to a fresh scan.
func (s *Services) appendMachineDesired(items []common.InstallationObservation, request MachineRestoreRequest) ([]common.InstallationObservation, error) {
	plan, err := s.planMachineRestoreScoped(request, false)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int)
	for i, item := range items {
		path := item.Path
		if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
			path = filepath.Join(parent, filepath.Base(path))
		}
		index[item.Target+"\x00"+path] = i
	}
	seen := make(map[string]bool)
	for _, batch := range plan.Plans {
		for _, desired := range batch.Items {
			key := desired.Target + "\x00" + desired.Path + "\x00" + desired.Skill
			if desired.Path == "" {
				key += "\x00" + desired.Directory
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			status := desired.Status
			if status == "install" || status == "unignored" {
				status = "missing"
			}
			if existing, ok := index[desired.Target+"\x00"+desired.Path]; ok && desired.Path != "" {
				item := &items[existing]
				item.Declared = true
				if status == "conflict" {
					if !slices.Contains(item.ExpectedSkills, desired.Skill) {
						item.ExpectedSkills = append(item.ExpectedSkills, desired.Skill)
					}
					if item.Status == "external" {
						item.Status = "conflict"
					}
					if desired.Error != "" {
						item.Error = desired.Error
					}
				}
				continue
			}
			if status == "installed" {
				continue
			}
			scope := "directory"
			if request.User {
				scope = "user"
			}
			directory := desired.Directory
			if len(request.Directories) == 1 {
				directory = request.Directories[0]
			}
			items = append(items, common.InstallationObservation{Scope: scope, Directory: directory, Target: desired.Target, Path: desired.Path, Skill: desired.Skill, Status: status, Error: desired.Error, Declared: true})
		}
	}
	return items, nil
}
