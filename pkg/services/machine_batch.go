package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MachineBatchRequest selects skills, clients and the union of directory tags.
type MachineBatchRequest struct {
	Skills         []string `json:"skills"`
	Targets        []string `json:"targets"`
	Tags           []string `json:"tags"`
	Directories    []string `json:"directories,omitempty"`
	User           bool     `json:"user,omitempty"`
	AllowUnignored bool     `json:"allow_unignored"`
}

type machineBatchDeclaration struct {
	Install common.Installation `json:"install"`
	Exists  bool                `json:"exists"`
}

type machineBatchSource struct {
	Skill   string `json:"skill"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// MachineBatchPlan is a read-only snapshot; paths in Items are never trusted
// as instructions when applying it.
type MachineBatchPlan struct {
	Action       string                        `json:"action"`
	Version      int                           `json:"version"`
	Request      MachineBatchRequest           `json:"request"`
	Directories  []MachineDirectoryInfo        `json:"directories"`
	Declarations []machineBatchDeclaration     `json:"declarations"`
	Relevant     []common.Installation         `json:"relevant_declarations,omitempty"`
	Sources      []machineBatchSource          `json:"sources"`
	Items        []common.InstallationPlanItem `json:"items"`
	Digest       string                        `json:"digest"`
}

func machineUniqueStrings(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return slices.Compact(out)
}

// PlanMachineBatch previews tag-scoped additions and their physical locations.
func (s *Services) PlanMachineBatch(request MachineBatchRequest) (*MachineBatchPlan, error) {
	request.Skills = machineUniqueStrings(request.Skills)
	request.Targets = machineUniqueStrings(request.Targets)
	request.Tags = machineUniqueStrings(request.Tags)
	request.Directories = machineUniqueStrings(request.Directories)
	scopes := 0
	if len(request.Tags) != 0 {
		scopes++
	}
	if len(request.Directories) != 0 {
		scopes++
	}
	if request.User {
		scopes++
	}
	if len(request.Skills) == 0 || len(request.Targets) == 0 || scopes != 1 {
		return nil, fmt.Errorf("select skills, targets and one scope type")
	}
	plan := &MachineBatchPlan{Action: "add", Version: 1, Request: request}
	var err error
	if request.User {
		plan.Directories = []MachineDirectoryInfo{}
	} else if len(request.Tags) != 0 {
		plan.Directories, err = s.SelectMachineDirectories(request.Tags)
	} else {
		var all []MachineDirectoryInfo
		all, err = s.ListDirectories()
		if err == nil {
			for _, id := range request.Directories {
				found := false
				for _, directory := range all {
					if directory.ID == id {
						plan.Directories = append(plan.Directories, directory)
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("unknown directory %q", id)
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	// Membership is represented by the selected IDs; unrelated tags do not
	// affect an already selected destination.
	for i := range plan.Directories {
		plan.Directories[i].Tags = nil
	}
	plan.Items, err = s.previewMachineBatchItems(request, plan.Directories)
	if err != nil {
		return nil, err
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return nil, err
	}
	plan.Relevant, err = s.machineBatchCoverage(doc, plan.Items)
	if err != nil {
		return nil, err
	}
	seen := make(map[common.Installation]bool)
	for _, skill := range request.Skills {
		entry, err := s.ResolveEntry(skill)
		if err != nil || entry == nil || entry.Kind != common.KindSkill || entry.Status != common.StatusActive {
			return nil, fmt.Errorf("active skill %q not found", skill)
		}
		ref := s.MachineSkillReference(entry)
		hash, err := dal.DirHash(entry.Path, nil)
		if err != nil {
			return nil, err
		}
		plan.Sources = append(plan.Sources, machineBatchSource{Skill: ref, Path: entry.Path, Content: hash})
		for _, target := range request.Targets {
			scopes := make([]common.InstallationScope, 0, len(request.Tags)+len(request.Directories))
			for _, tag := range request.Tags {
				scopes = append(scopes, common.InstallationScope{Kind: "tag", Name: tag})
			}
			for _, id := range request.Directories {
				scopes = append(scopes, common.InstallationScope{Kind: "directory", Name: id})
			}
			if request.User {
				scopes = append(scopes, common.InstallationScope{Kind: "user"})
			}
			for _, scope := range scopes {
				install := common.Installation{Skill: ref, Target: target, Scope: scope}
				if !seen[install] {
					seen[install] = true
					plan.Declarations = append(plan.Declarations, machineBatchDeclaration{Install: install, Exists: slices.Contains(doc.Installs, install)})
				}
			}
		}
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	plan.Digest = hex.EncodeToString(sum[:])
	return plan, nil
}
