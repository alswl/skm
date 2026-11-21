package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MachineDropPlan identifies an exact declaration to remove.
type MachineDropPlan struct {
	Action  string                `json:"action"`
	Version int                   `json:"version"`
	Install common.MachineInstall `json:"install"`
	Digest  string                `json:"digest"`
}

// ListMachineDeclarations returns current desired relationships.
func (s *Services) ListMachineDeclarations() ([]common.MachineInstall, error) {
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return nil, err
	}
	return doc.Installs, nil
}

// PlanMachineDrop previews removal of one existing declaration.
func (s *Services) PlanMachineDrop(install common.MachineInstall) (*MachineDropPlan, error) {
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return nil, err
	}
	found := false
	for _, existing := range doc.Installs {
		if existing == install {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("installation declaration not found")
	}
	plan := &MachineDropPlan{Action: "drop", Version: 1, Install: install}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	plan.Digest = hex.EncodeToString(sum[:])
	return plan, nil
}

// ApplyMachineDrop removes a declaration without modifying installed links.
func (s *Services) ApplyMachineDrop(ctx context.Context, plan MachineDropPlan) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	current, err := s.PlanMachineDrop(plan.Install)
	if err != nil {
		return err
	}
	if plan.Version != 1 || plan.Digest != current.Digest {
		return fmt.Errorf("drop plan is stale; create a new preview")
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return err
	}
	installs := make([]common.MachineInstall, 0, len(doc.Installs))
	for _, existing := range doc.Installs {
		if existing != plan.Install {
			installs = append(installs, existing)
		}
	}
	doc.Installs = installs
	return config.WriteMachineYAML(manifest, doc, 0o644)
}
