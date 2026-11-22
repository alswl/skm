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

// MachineTagMembersPlan previews a membership edit and the declarations it affects.
type MachineTagMembersPlan struct {
	Tag          string
	Before       []string
	After        []string
	Declarations []common.Installation
	Digest       string
}

func (s *Services) PlanMachineTagMembers(tag string, after []string) (*MachineTagMembersPlan, error) {
	manifest, _, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return nil, err
	}
	return planMachineTagMembers(doc, tag, after)
}

func planMachineTagMembers(doc config.InstallationsDocument, tag string, after []string) (*MachineTagMembersPlan, error) {
	if !directoryIDPattern.MatchString(tag) {
		return nil, fmt.Errorf("invalid tag %q", tag)
	}
	plan := &MachineTagMembersPlan{Tag: tag, After: slices.Clone(after)}
	slices.Sort(plan.After)
	plan.After = slices.Compact(plan.After)
	for _, id := range plan.After {
		if _, ok := doc.Directories[id]; !ok {
			return nil, fmt.Errorf("unknown directory %q", id)
		}
	}
	for id, dir := range doc.Directories {
		if containsMachineTag(dir.Tags, tag) {
			plan.Before = append(plan.Before, id)
		}
	}
	slices.Sort(plan.Before)
	for _, install := range doc.Installs {
		if install.Scope.Kind == "tag" && install.Scope.Name == tag {
			plan.Declarations = append(plan.Declarations, install)
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

// ApplyMachineTagMembers applies the reviewed membership and leaves existing links intact.
func (s *Services) ApplyMachineTagMembers(ctx context.Context, plan MachineTagMembersPlan) error {
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
	current, err := planMachineTagMembers(doc, plan.Tag, plan.After)
	if err != nil {
		return err
	}
	if current.Digest != plan.Digest {
		return fmt.Errorf("tag membership preview is stale; create a new preview")
	}
	for id, dir := range doc.Directories {
		member := containsMachineTag(current.After, id)
		present := containsMachineTag(dir.Tags, current.Tag)
		if member == present {
			continue
		}
		if member {
			dir.Tags = append(dir.Tags, current.Tag)
			slices.Sort(dir.Tags)
		} else {
			dir.Tags = slices.DeleteFunc(dir.Tags, func(tag string) bool { return tag == current.Tag })
		}
		doc.Directories[id] = dir
	}
	return config.WriteYAMLAtomic(manifest, doc, 0o644)
}
