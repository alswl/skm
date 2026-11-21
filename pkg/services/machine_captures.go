package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

type MachineCapturesRequest struct {
	Directory string   `json:"directory,omitempty"`
	Target    string   `json:"target"`
	Paths     []string `json:"paths"`
}

type MachineCapturesPlan struct {
	Action   string                   `json:"action"`
	Version  int                      `json:"version"`
	Request  MachineCapturesRequest   `json:"request"`
	Captures []MachineCapturePlan     `json:"captures"`
	Items    []common.MachinePlanItem `json:"items"`
	Digest   string                   `json:"digest"`
}

// PlanMachineCaptures independently inspects each explicitly selected link.
func (s *Services) PlanMachineCaptures(request MachineCapturesRequest) (*MachineCapturesPlan, error) {
	request.Paths = machineUniqueStrings(request.Paths)
	if request.Target == "" || len(request.Paths) == 0 {
		return nil, fmt.Errorf("select a target and at least one path")
	}
	if request.Directory == "" {
		if _, _, err := s.machineUserRoot(request.Target); err != nil {
			return nil, err
		}
	} else if request.Target != "codex" && request.Target != "claude" {
		return nil, fmt.Errorf("unsupported directory target %q", request.Target)
	}
	plan := &MachineCapturesPlan{Action: "capture", Version: 1, Request: request, Captures: []MachineCapturePlan{}, Items: []common.MachinePlanItem{}}
	for _, path := range request.Paths {
		item := common.MachinePlanItem{Directory: request.Directory, Target: request.Target, Path: path}
		capture, err := s.PlanMachineCapture(request.Directory, request.Target, path)
		if err != nil {
			item.Status, item.Error = "error", err.Error()
		} else {
			item.Path, item.Skill, item.Status = capture.Path, capture.Install.Skill, "capture"
			plan.Captures = append(plan.Captures, *capture)
		}
		plan.Items = append(plan.Items, item)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	plan.Digest = hex.EncodeToString(sum[:])
	return plan, nil
}

// ApplyMachineCaptures rechecks every selected path before merging declarations.
func (s *Services) ApplyMachineCaptures(ctx context.Context, selected MachineCapturesPlan) (*MachineBatchResult, error) {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	current, err := s.PlanMachineCaptures(selected.Request)
	if err != nil {
		return nil, err
	}
	if selected.Action != "capture" || selected.Version != 1 || selected.Digest != current.Digest {
		return nil, fmt.Errorf("capture plan is stale; create a new preview")
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return nil, err
	}
	changed := false
	for _, capture := range current.Captures {
		if !slices.Contains(doc.Installs, capture.Install) {
			doc.Installs = append(doc.Installs, capture.Install)
			changed = true
		}
	}
	if changed {
		if err := config.WriteMachineYAML(manifest, doc, 0o644); err != nil {
			return nil, err
		}
	}
	for i := range current.Items {
		if current.Items[i].Status == "capture" {
			current.Items[i].Status = "captured"
		}
	}
	return &MachineBatchResult{Items: current.Items}, nil
}
