package services

import (
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

// Explicit names take precedence over platforms so existing target selections
// stay stable when a platform has additional configured destinations.
func (s *Services) resolveTarget(kind common.EntryKind, name string) (common.InstallTarget, error) {
	if target, ok := s.Installer.TargetByName(name); ok {
		return target, nil
	}
	var candidates []common.InstallTarget
	var candidateNames []string
	foundPlatform := false
	for _, target := range s.Cfg.Targets {
		if name == "" || target.Platform != name {
			continue
		}
		foundPlatform = true
		if target.AcceptsKind(kind) {
			candidates = append(candidates, target)
			candidateNames = append(candidateNames, target.Name)
		}
	}
	switch len(candidates) {
	case 0:
		if foundPlatform {
			return common.InstallTarget{}, fmt.Errorf("platform %q does not accept a %s entry", name, kind)
		}
		return common.InstallTarget{}, fmt.Errorf("unknown target %q; use a target name or platform from 'skm target list'", name)
	case 1:
		return candidates[0], nil
	default:
		return common.InstallTarget{}, fmt.Errorf("platform %q is ambiguous for %s entries; select a target name: %s", name, kind, strings.Join(candidateNames, ", "))
	}
}

func (s *Services) resolveTargets(kind common.EntryKind, names []string) ([]common.InstallTarget, error) {
	var out []common.InstallTarget
	// A platform and its explicit target must not install the same entry twice.
	seen := map[string]bool{}
	for _, name := range names {
		target, err := s.resolveTarget(kind, name)
		if err != nil {
			return nil, err
		}
		if !seen[target.Name] {
			seen[target.Name] = true
			out = append(out, target)
		}
	}
	return out, nil
}
