package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MachineRestorePlan describes a fresh reconciliation of saved declarations.
type MachineRestorePlan struct {
	Action         string                      `json:"action"`
	Version        int                         `json:"version"`
	AllowUnignored bool                        `json:"allow_unignored"`
	Request        MachineRestoreRequest       `json:"request"`
	Selection      []string                    `json:"selection,omitempty"`
	Declarations   []common.MachineInstall     `json:"declarations"`
	Observations   []common.MachineObservation `json:"observations,omitempty"`
	Plans          []MachineBatchPlan          `json:"plans"`
	Digest         string                      `json:"digest"`
}

// MachineRestoreRequest limits reconciliation by physical target scope.
type MachineRestoreRequest struct {
	User           bool     `json:"user,omitempty"`
	Directories    []string `json:"directories,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	AllowUnignored bool     `json:"allow_unignored"`
}

// PlanMachineRestore reads the existing declarations without changing them.
func (s *Services) PlanMachineRestore(allowUnignored bool) (*MachineRestorePlan, error) {
	return s.PlanMachineRestoreScoped(MachineRestoreRequest{AllowUnignored: allowUnignored})
}

// PlanMachineRestoreScoped includes every declaration covering a selected location.
func (s *Services) PlanMachineRestoreScoped(request MachineRestoreRequest) (*MachineRestorePlan, error) {
	return s.planMachineRestoreScoped(request, true)
}

func (s *Services) planMachineRestoreScoped(request MachineRestoreRequest, includeActual bool) (*MachineRestorePlan, error) {
	request.Directories = machineUniqueStrings(request.Directories)
	request.Tags = machineUniqueStrings(request.Tags)
	if (request.User && (len(request.Tags) != 0 || len(request.Directories) != 0)) || (len(request.Tags) != 0 && len(request.Directories) != 0) {
		return nil, fmt.Errorf("select one restore scope type")
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return nil, err
	}
	plan := &MachineRestorePlan{Action: "restore", Version: 1, AllowUnignored: request.AllowUnignored, Request: request, Plans: []MachineBatchPlan{}, Declarations: []common.MachineInstall{}}
	unresolved := map[string]bool{}
	filtered := request.User || len(request.Directories) != 0 || len(request.Tags) != 0
	if len(request.Tags) != 0 {
		directories, err := s.SelectMachineDirectories(request.Tags)
		if err != nil {
			return nil, err
		}
		for _, directory := range directories {
			plan.Selection = append(plan.Selection, directory.ID)
		}
	} else if len(request.Directories) != 0 {
		for _, id := range request.Directories {
			if _, ok := doc.Directories[id]; !ok {
				return nil, fmt.Errorf("unknown directory %q", id)
			}
			plan.Selection = append(plan.Selection, id)
		}
	}
	declarations := append([]common.MachineInstall(nil), doc.Installs...)
	sort.Slice(declarations, func(i, j int) bool {
		a, b := declarations[i], declarations[j]
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
	for _, declaration := range declarations {
		batchRequest := MachineBatchRequest{Skills: []string{declaration.Skill}, Targets: []string{declaration.Target}, AllowUnignored: request.AllowUnignored}
		switch declaration.Scope.Kind {
		case "user":
			if filtered && !request.User {
				continue
			}
			batchRequest.User = true
		case "directory":
			if request.User || (filtered && !slices.Contains(plan.Selection, declaration.Scope.Name)) {
				continue
			}
			batchRequest.Directories = []string{declaration.Scope.Name}
		case "tag":
			if request.User {
				continue
			}
			if !filtered {
				batchRequest.Tags = []string{declaration.Scope.Name}
			} else {
				for _, id := range plan.Selection {
					if containsMachineTag(doc.Directories[id].Tags, declaration.Scope.Name) {
						batchRequest.Directories = append(batchRequest.Directories, id)
					}
				}
				if len(batchRequest.Directories) == 0 {
					continue
				}
			}
		default:
			return nil, fmt.Errorf("invalid declaration scope %q", declaration.Scope.Kind)
		}
		batch, err := s.PlanMachineBatch(batchRequest)
		if err != nil {
			unresolved[filepath.Base(declaration.Skill)] = true
			batch = &MachineBatchPlan{Action: "add", Version: 1, Request: batchRequest, Items: []common.MachinePlanItem{{Directory: declaration.Scope.Name, Target: declaration.Target, Skill: declaration.Skill, Status: "error", Error: err.Error()}}}
		}
		plan.Plans = append(plan.Plans, *batch)
		plan.Declarations = append(plan.Declarations, declaration)
	}
	// A physical location claimed by different skills cannot be installed.
	claims := make(map[string]map[string]bool)
	for _, batch := range plan.Plans {
		for _, item := range batch.Items {
			if item.Path == "" {
				continue
			}
			if claims[item.Path] == nil {
				claims[item.Path] = make(map[string]bool)
			}
			claims[item.Path][item.Skill] = true
		}
	}
	for i := range plan.Plans {
		for j := range plan.Plans[i].Items {
			item := &plan.Plans[i].Items[j]
			if len(claims[item.Path]) > 1 {
				item.Status, item.Error = "conflict", "multiple skills select this location"
			} else if item.Path != "" && unresolved[filepath.Base(item.Path)] {
				item.Status, item.Error = "conflict", "an unresolved declaration may select this location"
			}
		}
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	plan.Digest = hex.EncodeToString(sum[:])
	if includeActual {
		plan.Observations, err = s.scanMachineUndeclared(request, plan.Selection)
		if err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func (s *Services) scanMachineUndeclared(request MachineRestoreRequest, selection []string) ([]common.MachineObservation, error) {
	var result []common.MachineObservation
	appendUndeclared := func(items []common.MachineObservation) {
		for _, item := range items {
			if (item.Status == "installed" && !item.Declared) || item.Status == "error" || item.Status == "recovery" {
				result = append(result, item)
			}
		}
	}
	filtered := request.User || len(request.Directories) != 0 || len(request.Tags) != 0
	if !filtered || request.User {
		items, err := s.scanMachineUser(false)
		if err != nil {
			return nil, err
		}
		appendUndeclared(items)
	}
	if request.User {
		return result, nil
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		if filtered && !slices.Contains(selection, directory.ID) {
			continue
		}
		if directory.Path == "" {
			continue
		}
		info, err := os.Stat(directory.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		items, err := s.scanMachinePath(directory.ID, directory.Path, false)
		if err != nil {
			return nil, err
		}
		appendUndeclared(items)
	}
	return result, nil
}

// ApplyMachineRestore checks the complete plan before attempting any link.
func (s *Services) ApplyMachineRestore(ctx context.Context, selected MachineRestorePlan) (*MachineBatchResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	current, err := s.PlanMachineRestoreScoped(selected.Request)
	if err != nil {
		return nil, err
	}
	if selected.Action != "restore" || selected.Version != 1 || selected.Digest != current.Digest {
		return nil, fmt.Errorf("machine plan is stale; create a new preview")
	}
	result := &MachineBatchResult{Items: []common.MachinePlanItem{}}
	seen := make(map[string]bool)
	for i := range current.Plans {
		batchResult := s.executeMachineBatchItems(&current.Plans[i], seen)
		for _, item := range batchResult.Items {
			if item.Status != "duplicate" {
				result.Items = append(result.Items, item)
			}
		}
	}
	return result, nil
}
