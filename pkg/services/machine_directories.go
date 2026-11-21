package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

var directoryIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// MachineDirectoryInfo describes a registered directory on this machine.
type MachineDirectoryInfo struct {
	ID   string   `json:"id"`
	Path string   `json:"path,omitempty"`
	Tags []string `json:"tags"`
}

// MachineDiscovery identifies a found root or a failed candidate.
type MachineDiscovery struct {
	Path       string `json:"path"`
	Directory  string `json:"directory,omitempty"`
	Registered bool   `json:"registered"`
	Error      string `json:"error,omitempty"`
}

// DiscoverDirectories finds Git roots beneath the selected parent.
func (s *Services) DiscoverDirectories(ctx context.Context, parent string) ([]string, error) {
	return engines.DiscoverWorktrees(ctx, parent)
}

// DiscoverDirectoriesReport also reports registered roots and failed candidates.
func (s *Services) DiscoverDirectoriesReport(ctx context.Context, parent string) ([]MachineDiscovery, error) {
	found, err := engines.DiscoverWorktreesReport(ctx, parent)
	if err != nil {
		return nil, err
	}
	directories, err := s.ListDirectories()
	if err != nil {
		return nil, err
	}
	registered := make(map[string]string)
	for _, directory := range directories {
		if directory.Path != "" {
			registered[directory.Path] = directory.ID
		}
	}
	result := make([]MachineDiscovery, 0, len(found))
	for _, item := range found {
		id := registered[item.Path]
		result = append(result, MachineDiscovery{Path: item.Path, Directory: id, Registered: id != "", Error: item.Error})
	}
	return result, nil
}

func (s *Services) machineFiles() (string, string, error) {
	paths, err := config.MachinePathsFile(s.Cfg.ConfigDir, s.Cfg.Root)
	return filepath.Join(s.Cfg.Root, "machine.yaml"), paths, err
}

// ListDirectories reads the declaration and machine-local paths.
func (s *Services) ListDirectories() ([]MachineDirectoryInfo, error) {
	manifestPath, pathsPath, err := s.machineFiles()
	if err != nil {
		return nil, err
	}
	doc, err := config.LoadMachineDocument(manifestPath)
	if err != nil {
		return nil, err
	}
	paths, err := config.LoadMachinePaths(pathsPath, doc)
	if err != nil {
		return nil, err
	}
	result := make([]MachineDirectoryInfo, 0, len(doc.Directories))
	for id, entry := range doc.Directories {
		result = append(result, MachineDirectoryInfo{ID: id, Path: paths.Paths[id], Tags: entry.Tags})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// RegisterDirectories registers Git roots and their tags in a single batch.
func (s *Services) RegisterDirectories(ctx context.Context, directories map[string]string, tags []string) error {
	return s.registerDirectories(ctx, directories, tags, config.WriteMachineYAML)
}

func (s *Services) registerDirectories(ctx context.Context, directories map[string]string, tags []string, write func(string, any, os.FileMode) error) error {
	if len(directories) == 0 {
		return fmt.Errorf("no directories selected")
	}
	for _, tag := range tags {
		if !directoryIDPattern.MatchString(tag) {
			return fmt.Errorf("invalid tag %q", tag)
		}
	}
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	manifestPath, pathsPath, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadMachineDocument(manifestPath)
	if err != nil {
		return err
	}
	paths, err := config.LoadMachinePaths(pathsPath, doc)
	if err != nil {
		return err
	}
	if doc.Directories == nil {
		doc.Directories = make(map[string]common.MachineDirectory)
	}
	if paths.Paths == nil {
		paths.Paths = make(map[string]string)
	}
	for id, path := range directories {
		if !directoryIDPattern.MatchString(id) {
			return fmt.Errorf("invalid directory ID %q", id)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "git", "-C", canonical, "rev-parse", "--show-toplevel")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("directory %q is not a Git worktree: %w", path, err)
		}
		gitRoot, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
		if err != nil || gitRoot != canonical {
			return fmt.Errorf("directory %q is not a Git worktree root", path)
		}
		for other, existing := range paths.Paths {
			if other != id && existing == canonical {
				return fmt.Errorf("directory %q already registered as %q", canonical, other)
			}
		}
		if existing := paths.Paths[id]; existing != "" && existing != canonical {
			return fmt.Errorf("directory %q already has a different path", id)
		}
		entry := doc.Directories[id]
		for _, tag := range tags {
			if !containsMachineTag(entry.Tags, tag) {
				entry.Tags = append(entry.Tags, tag)
			}
		}
		sort.Strings(entry.Tags)
		doc.Directories[id] = entry
		paths.Paths[id] = canonical
	}
	if err := write(manifestPath, doc, 0o644); err != nil {
		return err
	}
	if err := write(pathsPath, paths, 0o600); err != nil {
		return fmt.Errorf("directories registered, but local paths could not be saved; retry: %w", err)
	}
	return nil
}

func containsMachineTag(tags []string, tag string) bool {
	for _, existing := range tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// TagDirectories updates the tags of registered directories without changing installations.
func (s *Services) TagDirectories(ctx context.Context, ids, add, remove []string) error {
	for _, tag := range append(append([]string{}, add...), remove...) {
		if !directoryIDPattern.MatchString(tag) {
			return fmt.Errorf("invalid tag %q", tag)
		}
		if containsMachineTag(add, tag) && containsMachineTag(remove, tag) {
			return fmt.Errorf("tag %q cannot be added and removed together", tag)
		}
	}
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	manifest, _, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadMachineDocument(manifest)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := doc.Directories[id]; !ok {
			return fmt.Errorf("unknown directory %q", id)
		}
	}
	for _, id := range ids {
		entry := doc.Directories[id]
		var tags []string
		for _, tag := range entry.Tags {
			if !containsMachineTag(remove, tag) && !containsMachineTag(tags, tag) {
				tags = append(tags, tag)
			}
		}
		for _, tag := range add {
			if !containsMachineTag(tags, tag) {
				tags = append(tags, tag)
			}
		}
		sort.Strings(tags)
		entry.Tags = tags
		doc.Directories[id] = entry
	}
	return config.WriteMachineYAML(manifest, doc, 0o644)
}

// SetDirectoryPath associates a registered directory with a local Git root.
func (s *Services) SetDirectoryPath(ctx context.Context, id, path string) error {
	lock, err := dal.AcquireLock(ctx, s.Cfg.Root)
	if err != nil {
		return err
	}
	defer lock.Release()
	manifestPath, pathsPath, err := s.machineFiles()
	if err != nil {
		return err
	}
	doc, err := config.LoadMachineDocument(manifestPath)
	if err != nil {
		return err
	}
	if _, ok := doc.Directories[id]; !ok {
		return fmt.Errorf("unknown directory %q", id)
	}
	paths, err := config.LoadMachinePaths(pathsPath, doc)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, "git", "-C", canonical, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("directory %q is not a Git worktree: %w", path, err)
	}
	gitRoot, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil || gitRoot != canonical {
		return fmt.Errorf("directory %q is not a Git worktree root", path)
	}
	for other, existing := range paths.Paths {
		if other != id && existing == canonical {
			return fmt.Errorf("directory %q already registered as %q", canonical, other)
		}
	}
	if paths.Paths == nil {
		paths.Paths = make(map[string]string)
	}
	paths.Paths[id] = canonical
	return config.WriteMachineYAML(pathsPath, paths, 0o600)
}
