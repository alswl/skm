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

// MachineTagExitPlan removes one directory from one skill and client declaration.
type MachineTagExitPlan struct {
	Install     common.Installation
	Directory   string
	Remaining   []string
	Path        string
	Coverage    []common.Installation
	CoverageGap string
	Digest      string
}

func (s *Services) PlanMachineTagExit(install common.Installation, directory, path string) (*MachineTagExitPlan, error) {
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return nil, err
	}
	return s.planMachineTagExit(doc, install, directory, path)
}

func (s *Services) planMachineTagExit(doc config.InstallationsDocument, install common.Installation, directory, path string) (*MachineTagExitPlan, error) {
	if install.Scope.Kind != "tag" || !slices.Contains(doc.Installs, install) {
		return nil, fmt.Errorf("tag installation declaration not found")
	}
	if !containsMachineTag(doc.Directories[directory].Tags, install.Scope.Name) {
		return nil, fmt.Errorf("directory %q is not a member of tag %q", directory, install.Scope.Name)
	}
	plan := &MachineTagExitPlan{Install: install, Directory: directory, Path: path}
	for id, entry := range doc.Directories {
		if id != directory && containsMachineTag(entry.Tags, install.Scope.Name) {
			plan.Remaining = append(plan.Remaining, id)
		}
	}
	slices.Sort(plan.Remaining)
	if path != "" {
		coverage, err := s.MachineLocationCoverage(path)
		plan.Coverage = slices.DeleteFunc(coverage, func(item common.Installation) bool { return item == install })
		if err != nil {
			plan.CoverageGap = err.Error()
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

// ApplyMachineTagExit changes declarations atomically and leaves links and
// directory tags unchanged.
func (s *Services) ApplyMachineTagExit(ctx context.Context, plan MachineTagExitPlan) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	manifest, _, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return err
	}
	current, err := s.planMachineTagExit(doc, plan.Install, plan.Directory, plan.Path)
	if err != nil {
		return fmt.Errorf("tag exit preview is stale: %w", err)
	}
	if current.Digest != plan.Digest {
		return fmt.Errorf("tag exit preview is stale; create a new preview")
	}
	doc.Installs = slices.DeleteFunc(doc.Installs, func(item common.Installation) bool { return item == current.Install })
	for _, id := range current.Remaining {
		item := current.Install
		item.Scope = common.InstallationScope{Kind: "directory", Name: id}
		if !slices.Contains(doc.Installs, item) {
			doc.Installs = append(doc.Installs, item)
		}
	}
	return config.WriteYAMLAtomic(manifest, doc, 0o644)
}
