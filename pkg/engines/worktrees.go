package engines

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// WorktreeDiscovery is either a Git root or a diagnostic for a candidate.
type WorktreeDiscovery struct {
	Path  string
	Error string
}

// DiscoverWorktrees finds Git roots, omitting individual discovery errors.
func DiscoverWorktrees(ctx context.Context, parent string) ([]string, error) {
	report, err := DiscoverWorktreesReport(ctx, parent)
	if err != nil {
		return nil, err
	}
	found := []string{}
	for _, item := range report {
		if item.Error == "" {
			found = append(found, item.Path)
		}
	}
	return found, nil
}

// DiscoverWorktreesReport keeps individual failures while continuing siblings.
func DiscoverWorktreesReport(ctx context.Context, parent string) ([]WorktreeDiscovery, error) {
	root, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("discovery parent %q is not a directory", parent)
	}
	found := []WorktreeDiscovery{}
	seen := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			found = append(found, WorktreeDiscovery{Path: path, Error: walkErr.Error()})
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			found = append(found, WorktreeDiscovery{Path: path, Error: err.Error()})
			return nil
		}
		cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				found = append(found, WorktreeDiscovery{Path: path, Error: "cannot inspect Git root: " + err.Error()})
				return nil
			}
			return fmt.Errorf("inspect Git directory %q: %w", path, err)
		}
		worktree, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
		if err != nil {
			found = append(found, WorktreeDiscovery{Path: path, Error: err.Error()})
			return nil
		}
		if worktree == path && !seen[worktree] {
			seen[worktree] = true
			found = append(found, WorktreeDiscovery{Path: worktree})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	return found, nil
}

// MachineGitSlotStatus reports whether a destination is tracked, ignored, or
// unignored. Git errors are propagated instead of treated as an empty result.
func MachineGitSlotStatus(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("destination outside directory")
	}
	tracked, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", rel).Output()
	if err != nil {
		return "", err
	}
	if len(tracked) != 0 {
		return "conflict", nil
	}
	cmd := exec.Command("git", "-C", root, "check-ignore", "-q", "--", rel)
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return "unignored", nil
		}
		return "", err
	}
	return "install", nil
}
