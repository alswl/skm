package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// AdoptResult reports the outcome of adopting an external skill (FR-038).
type AdoptResult struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Path   string `json:"path"`
}

// AdoptExternal turns a real, unmanaged external skill directory (as reported
// by discover) into a managed repository entry: it imports the directory into
// the repo local layer, then reinstalls it as a managed symlink into the target
// that held it, replacing the real directory (FR-038). A symlink is never
// adopted — discover only reports real directories, and this guards again so a
// symlink is never removed implicitly.
func (s *Services) AdoptExternal(ctx context.Context, path string, force bool) (*AdoptResult, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("adopt: %w", err), common.ExitError)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, common.WithExitCode(fmt.Errorf("adopt: %q is a symlink, not an external skill", path), common.ExitError)
	}

	target, err := s.targetContaining(path)
	if err != nil {
		return nil, err
	}

	// Import as a local asset (copy into repo, no origin).
	imp, err := s.Import(ctx, path, ImportOptions{Force: force})
	if err != nil {
		return nil, err
	}

	// Replace the real external directory with a managed symlink into its target.
	if _, err := s.Install(ctx, imp.Name, InstallOptions{Targets: []string{target.Name}, Force: true}); err != nil {
		return nil, err
	}

	return &AdoptResult{Name: imp.Name, Target: target.Name, Path: imp.Path}, nil
}

// DeleteExternal permanently removes a real external skill directory in a
// target (FR-038). It refuses symlinks so a managed install is never removed by
// this path, and requires the caller (TUI) to have obtained explicit
// confirmation.
func (s *Services) DeleteExternal(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return common.WithExitCode(fmt.Errorf("delete-external: %w", err), common.ExitError)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return common.WithExitCode(fmt.Errorf("delete-external: %q is a symlink; refusing", path), common.ExitError)
	}
	if _, err := s.targetContaining(path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return common.WithExitCode(fmt.Errorf("delete-external: %w", err), common.ExitError)
	}
	return nil
}

// targetContaining returns the skill target directory that directly holds path.
func (s *Services) targetContaining(path string) (common.InstallTarget, error) {
	parent := filepath.Dir(filepath.Clean(path))
	for _, t := range s.Cfg.Targets {
		if t.Kind == common.KindSkill && filepath.Clean(t.Path) == parent {
			return t, nil
		}
	}
	return common.InstallTarget{}, common.WithExitCode(
		fmt.Errorf("adopt: %q is not inside a known skill target", path), common.ExitError)
}
