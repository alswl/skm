package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

// ErrMachineUnlinkCovered means a declaration still protects the selected link.
var ErrMachineUnlinkCovered = errors.New("selected link remains covered by a declaration")

// ErrMachineUnlinkCoverageUnknown means declaration coverage cannot be established.
var ErrMachineUnlinkCoverageUnknown = errors.New("cannot establish declaration coverage")

// MachineUnlinkPlan identifies a current link chosen for removal.
type MachineUnlinkPlan struct {
	Action    string                  `json:"action"`
	Version   int                     `json:"version"`
	Directory string                  `json:"directory,omitempty"`
	Target    string                  `json:"target"`
	Path      string                  `json:"path"`
	Skill     string                  `json:"skill"`
	Content   string                  `json:"content"`
	Identity  dal.MachineLinkIdentity `json:"identity"`
	Digest    string                  `json:"digest"`
}

func (s *Services) machineUnlinkRoot(directory, target string) (string, error) {
	if directory == "" {
		root, _, err := s.machineUserRoot(target)
		return root, err
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return "", err
	}
	for _, item := range directories {
		if item.ID == directory {
			if item.Path == "" {
				return "", fmt.Errorf("directory %q has no local path", directory)
			}
			return item.Path, nil
		}
	}
	return "", fmt.Errorf("unknown directory %q", directory)
}

// PlanMachineUnlink validates a current link and requires complete, uncovered state.
// An empty directory selects a user-level target.
func (s *Services) PlanMachineUnlink(directory, target, path string) (*MachineUnlinkPlan, error) {
	capture, err := s.PlanMachineCapture(directory, target, path)
	if err != nil {
		return nil, err
	}
	root, err := s.machineUnlinkRoot(directory, target)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, capture.Path)
	if err != nil {
		return nil, err
	}
	if directory != "" {
		status, err := engines.MachineGitSlotStatus(root, capture.Path)
		if err != nil {
			return nil, err
		}
		if status == "conflict" {
			return nil, fmt.Errorf("selected link is tracked by Git")
		}
	}
	identity, err := dal.InspectMachineLink(root, rel)
	if err != nil {
		return nil, err
	}
	if identity.Target != capture.Link {
		return nil, fmt.Errorf("selected link changed during preview")
	}
	if err := s.checkMachineUnlinkCoverage(capture.Path); err != nil {
		return nil, err
	}
	plan := &MachineUnlinkPlan{Action: "unlink", Version: 1, Directory: directory, Target: target, Path: capture.Path, Skill: capture.Install.Skill, Content: capture.Content, Identity: identity}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	plan.Digest = hex.EncodeToString(sum[:])
	return plan, nil
}

// checkMachineUnlinkCoverage inspects every declaration whose installation
// name could identify the selected position. Unrelated skills do not prevent
// unlink when their source or target is unavailable.
func (s *Services) checkMachineUnlinkCoverage(path string) error {
	doc, err := config.LoadInstallations(filepath.Join(s.Cfg.Root, config.InstallationsFile))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMachineUnlinkCoverageUnknown, err)
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMachineUnlinkCoverageUnknown, err)
	}
	name := filepath.Base(path)
	for _, declaration := range doc.Installs {
		if filepath.Base(declaration.Skill) != name {
			continue
		}
		entry, err := s.ResolveEntry(declaration.Skill)
		if err != nil || entry == nil || entry.Name != name {
			return fmt.Errorf("%w for %q", ErrMachineUnlinkCoverageUnknown, declaration.Skill)
		}
		if declaration.Scope.Kind == "user" {
			_, target, err := s.machineUserRoot(declaration.Target)
			if err != nil {
				return fmt.Errorf("%w for target %q: %w", ErrMachineUnlinkCoverageUnknown, declaration.Target, err)
			}
			if filepath.Join(target, name) == path {
				return fmt.Errorf("%w: %s on user target %q", ErrMachineUnlinkCovered, declaration.Skill, declaration.Target)
			}
			continue
		}
		subdir := ".agents"
		if declaration.Target == "claude" {
			subdir = ".claude"
		}
		for _, directory := range directories {
			selected := declaration.Scope.Kind == "directory" && declaration.Scope.Name == directory.ID || declaration.Scope.Kind == "tag" && containsMachineTag(directory.Tags, declaration.Scope.Name)
			if !selected {
				continue
			}
			if directory.Path == "" {
				return fmt.Errorf("%w for directory %q: missing path", ErrMachineUnlinkCoverageUnknown, directory.ID)
			}
			info, err := os.Stat(directory.Path)
			if err != nil || !info.IsDir() {
				return fmt.Errorf("%w for directory %q", ErrMachineUnlinkCoverageUnknown, directory.ID)
			}
			candidate := filepath.Join(directory.Path, subdir, "skills", name)
			parent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("%w for directory %q: %w", ErrMachineUnlinkCoverageUnknown, directory.ID, err)
			}
			if err == nil {
				candidate = filepath.Join(parent, name)
			}
			if candidate == path {
				if declaration.Scope.Kind == "tag" {
					count := 0
					for _, dir := range directories {
						if containsMachineTag(dir.Tags, declaration.Scope.Name) {
							count++
						}
					}
					return fmt.Errorf("%w: tag %q (%d matching directories), %s on %s", ErrMachineUnlinkCovered, declaration.Scope.Name, count, declaration.Skill, declaration.Target)
				}
				return fmt.Errorf("%w: %s on %s in directory %q", ErrMachineUnlinkCovered, declaration.Skill, declaration.Target, declaration.Scope.Name)
			}
		}
	}
	return nil
}

// ApplyMachineUnlink repeats all checks and removes only the seized object.
func (s *Services) ApplyMachineUnlink(ctx context.Context, selected MachineUnlinkPlan) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	current, err := s.PlanMachineUnlink(selected.Directory, selected.Target, selected.Path)
	if err != nil {
		return err
	}
	if selected.Action != "unlink" || selected.Version != 1 || selected.Digest != current.Digest {
		return fmt.Errorf("unlink plan is stale; create a new preview")
	}
	root, err := s.machineUnlinkRoot(current.Directory, current.Target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, current.Path)
	if err != nil {
		return err
	}
	return dal.RemoveMachineLink(root, rel, current.Identity)
}
