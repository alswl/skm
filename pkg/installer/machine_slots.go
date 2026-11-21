package installer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// MachineTarget describes one physical skills directory.
type MachineTarget struct {
	Name string
	Path string
}

// MachineDirectoryTargets returns the supported project-level locations.
func MachineDirectoryTargets(root string) []MachineTarget {
	return []MachineTarget{{Name: "codex", Path: filepath.Join(root, ".agents", "skills")}, {Name: "claude", Path: filepath.Join(root, ".claude", "skills")}}
}

// MachineUserTargets returns configured targets that use direct skill links.
func MachineUserTargets(targets []common.InstallTarget) []MachineTarget {
	out := make([]MachineTarget, 0, len(targets))
	for _, target := range targets {
		strategy, ok := target.EffectiveStrategy(common.KindSkill)
		if target.AcceptsKind(common.KindSkill) && ok && strategy == common.StrategySkillSymlink {
			out = append(out, MachineTarget{Name: target.Name, Path: target.Path})
		}
	}
	return out
}

// MachineSource identifies an active library entry by its physical path.
type MachineSource struct {
	Path  string
	Skill string
}

// MachineSourceDiagnostic identifies a source that could not be matched safely.
type MachineSourceDiagnostic struct {
	Path  string
	Skill string
	Error string
}

// MachineSourceIndex canonicalizes paths and suppresses ambiguous identities.
func MachineSourceIndex(sources []MachineSource) (map[string]string, error) {
	index, diagnostics := InspectMachineSources(sources)
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("%s", diagnostics[0].Error)
	}
	return index, nil
}

// InspectMachineSources keeps valid sources available when another fails.
func InspectMachineSources(sources []MachineSource) (map[string]string, []MachineSourceDiagnostic) {
	index := make(map[string]string, len(sources))
	ambiguous := make(map[string]bool)
	var diagnostics []MachineSourceDiagnostic
	for _, source := range sources {
		path, err := filepath.EvalSymlinks(source.Path)
		if err != nil {
			diagnostics = append(diagnostics, MachineSourceDiagnostic{Path: source.Path, Skill: source.Skill, Error: fmt.Sprintf("inspect skill %q: %s", source.Skill, err)})
			continue
		}
		if old, ok := index[path]; ok && old != source.Skill {
			ambiguous[path] = true
			diagnostics = append(diagnostics, MachineSourceDiagnostic{Path: path, Error: fmt.Sprintf("multiple skills identify source %q", path)})
		}
		index[path] = source.Skill
	}
	for path := range ambiguous {
		delete(index, path)
	}
	return index, diagnostics
}

// MachineSlot is an observation of a single location, never stored as history.
type MachineSlot struct {
	Status string
	Skill  string
	Error  string
}

// InspectMachineSlot distinguishes absent, foreign, broken and matching links.
func InspectMachineSlot(path string, sources map[string]string) MachineSlot {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return MachineSlot{Status: "missing"}
	}
	if err != nil {
		return MachineSlot{Status: "error", Error: err.Error()}
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return MachineSlot{Status: "external"}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return MachineSlot{Status: "dangling"}
	}
	if err != nil {
		return MachineSlot{Status: "error", Error: err.Error()}
	}
	if skill := sources[resolved]; skill != "" {
		return MachineSlot{Status: "installed", Skill: skill}
	}
	return MachineSlot{Status: "external"}
}
