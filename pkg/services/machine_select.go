package services

import (
	"fmt"
	"sort"
)

// SelectMachineDirectories selects registered directories by tag union.
// An empty tag list selects no directories.
func (s *Services) SelectMachineDirectories(tags []string) ([]MachineDirectoryInfo, error) {
	for _, tag := range tags {
		if !directoryIDPattern.MatchString(tag) {
			return nil, fmt.Errorf("invalid tag %q", tag)
		}
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	selected := make([]MachineDirectoryInfo, 0)
	for _, directory := range directories {
		for _, tag := range tags {
			if containsMachineTag(directory.Tags, tag) {
				selected = append(selected, directory)
				break
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	return selected, nil
}
