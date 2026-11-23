package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MachineCapturePlan describes one selected existing installation.
type MachineCapturePlan struct {
	Action    string              `json:"action"`
	Version   int                 `json:"version"`
	Directory string              `json:"directory"`
	Target    string              `json:"target"`
	Path      string              `json:"path"`
	Install   common.Installation `json:"install"`
	Digest    string              `json:"digest"`
	Link      string              `json:"link"`
	Content   string              `json:"content"`
	Declared  bool                `json:"declared"`
}

// PlanMachineCapture checks that the selected path is a matching library link.
// An empty directory selects configured user-level targets.
func (s *Services) PlanMachineCapture(directory, target, path string) (*MachineCapturePlan, error) {
	var items []common.InstallationObservation
	var err error
	if directory == "" {
		items, err = s.ScanMachineUser()
	} else {
		items, err = s.ScanMachineDirectory(directory)
	}
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		absolute = filepath.Join(parent, filepath.Base(absolute))
	}
	for _, item := range items {
		itemPath, err := filepath.Abs(item.Path)
		if err != nil {
			continue
		}
		if parent, err := filepath.EvalSymlinks(filepath.Dir(itemPath)); err == nil {
			itemPath = filepath.Join(parent, filepath.Base(itemPath))
		}
		if itemPath != absolute || item.Target != target {
			continue
		}
		if item.Status != "installed" || item.Skill == "" {
			return nil, fmt.Errorf("selected installation is not a matching library link")
		}
		scope := common.InstallationScope{Kind: "directory", Name: directory}
		if directory == "" {
			scope = common.InstallationScope{Kind: "user"}
		}
		plan := &MachineCapturePlan{
			Action: "capture", Version: 1, Directory: directory, Target: target, Path: absolute,
			Install: common.Installation{
				Skill: item.Skill, Target: target,
				Scope: scope,
			},
		}
		before, err := os.Lstat(absolute)
		if err != nil || before.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("selected link changed during preview")
		}
		plan.Link, err = os.Readlink(absolute)
		if err != nil {
			return nil, err
		}
		entry, err := s.ResolveEntry(item.Skill)
		if err != nil || entry == nil {
			return nil, fmt.Errorf("cannot resolve skill %q", item.Skill)
		}
		plan.Content, err = dal.DirHash(entry.Path, nil)
		if err != nil {
			return nil, err
		}
		linkTarget, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("inspect selected link: %w", err)
		}
		source, err := filepath.EvalSymlinks(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("inspect selected skill: %w", err)
		}
		after, err := os.Lstat(absolute)
		if err != nil || !os.SameFile(before, after) || linkTarget != source {
			return nil, fmt.Errorf("selected link changed during preview")
		}
		manifest, _, err := s.machineFiles()
		if err != nil {
			return nil, err
		}
		doc, err := config.LoadInstallations(manifest)
		if err != nil {
			return nil, err
		}
		for _, existing := range doc.Installs {
			if existing == plan.Install {
				plan.Declared = true
				break
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
	return nil, fmt.Errorf("installation %q not found in target %q", path, target)
}

// ApplyMachineCapture rechecks the selected link and adds its declaration.
func (s *Services) ApplyMachineCapture(ctx context.Context, plan MachineCapturePlan) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	current, err := s.PlanMachineCapture(plan.Directory, plan.Target, plan.Path)
	if err != nil {
		return err
	}
	if plan.Version != 1 || current.Digest != plan.Digest || current.Install != plan.Install {
		return fmt.Errorf("capture plan is stale; create a new preview")
	}
	manifest, _, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadInstallations(manifest)
	if err != nil {
		return err
	}
	for _, existing := range doc.Installs {
		if existing == current.Install {
			return nil
		}
	}
	doc.Installs = append(doc.Installs, current.Install)
	return config.WriteYAMLAtomic(manifest, doc, 0o644)
}
