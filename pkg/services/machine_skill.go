package services

import "github.com/alswl/skm/skm/pkg/common"

// MachineSkillReference keeps declarations tied to the upstream identity while
// the active content may come from a fork.
func (s *Services) MachineSkillReference(entry *common.Entry) string {
	if entry.UpstreamPath != "" {
		return s.Repo.RelPath(entry.UpstreamPath)
	}
	return s.Repo.RelPath(entry.Path)
}
