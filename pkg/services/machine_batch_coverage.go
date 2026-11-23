package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
)

// machineBatchCoverage incorporates declarations that cover selected physical
// positions, including those with another scope or target alias.
func (s *Services) machineBatchCoverage(doc config.InstallationsDocument, items []common.InstallationPlanItem) ([]common.Installation, error) {
	byPath := make(map[string][]int)
	names := make(map[string]bool)
	for i, item := range items {
		if item.Path != "" {
			byPath[item.Path] = append(byPath[item.Path], i)
			names[filepath.Base(item.Path)] = true
		}
	}
	if len(byPath) == 0 {
		return nil, nil
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	var relevant []common.Installation
	for _, install := range doc.Installs {
		if !names[filepath.Base(install.Skill)] {
			continue
		}
		entry, err := s.ResolveEntry(install.Skill)
		if err != nil || entry == nil {
			return nil, fmt.Errorf("cannot resolve potentially overlapping declaration %q", install.Skill)
		}
		paths := []string{}
		switch install.Scope.Kind {
		case "user":
			_, target, err := s.machineUserRoot(install.Target)
			if err != nil {
				return nil, fmt.Errorf("cannot inspect potentially overlapping target %q: %w", install.Target, err)
			}
			paths = append(paths, filepath.Join(target, entry.Name))
		case "directory", "tag":
			subdir := ".agents"
			if install.Target == "claude" {
				subdir = ".claude"
			}
			for _, directory := range directories {
				selected := install.Scope.Kind == "directory" && install.Scope.Name == directory.ID || install.Scope.Kind == "tag" && containsMachineTag(directory.Tags, install.Scope.Name)
				if selected && directory.Path != "" {
					path := filepath.Join(directory.Path, subdir, "skills", entry.Name)
					parent, err := filepath.EvalSymlinks(filepath.Dir(path))
					if err != nil && !os.IsNotExist(err) {
						return nil, fmt.Errorf("cannot inspect declaration for directory %q: %w", directory.ID, err)
					}
					if err == nil {
						path = filepath.Join(parent, entry.Name)
					}
					paths = append(paths, path)
				}
			}
		}
		matched := false
		for _, path := range paths {
			for _, index := range byPath[path] {
				matched = true
				if items[index].Skill != s.MachineSkillReference(entry) {
					items[index].Status = "conflict"
					items[index].Error = "another declaration selects this location for a different skill"
				}
			}
		}
		if matched {
			relevant = append(relevant, install)
		}
	}
	sort.Slice(relevant, func(i, j int) bool {
		a, b := relevant[i], relevant[j]
		if a.Skill != b.Skill {
			return a.Skill < b.Skill
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Scope.Kind != b.Scope.Kind {
			return a.Scope.Kind < b.Scope.Kind
		}
		return a.Scope.Name < b.Scope.Name
	})
	return relevant, nil
}
