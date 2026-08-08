package services

import (
	"github.com/alswl/skm/skm/pkg/common"
)

// DiscoverResult is the CLI JSON report for discover (contract/cli-json.md).
type DiscoverResult struct {
	Source string            `json:"source"`
	Found  []DiscoveredSkill `json:"found"`
}

// Discover lists external unmanaged skills. sourceDir restricts the scan to
// one directory; when empty all skill-kind targets are scanned.
func (s *Services) Discover(sourceDir string) *DiscoverResult {
	found := s.Repo.Discover(s.Cfg.Targets, sourceDir)
	src := sourceDir
	if src == "" {
		for _, t := range s.Cfg.Targets {
			if t.AcceptsKind(common.KindSkill) {
				src = t.Path
				break
			}
		}
	}
	return &DiscoverResult{Source: src, Found: found}
}
