package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

// LifecycleResult is the CLI JSON report for lifecycle commands.
type LifecycleResult struct {
	Action  string            `json:"action"`
	Name    string            `json:"name"`
	DryRun  bool              `json:"dry_run"`
	Path    string            `json:"path"`
	Type    *common.EntryKind `json:"type,omitempty"`
	Success bool              `json:"success"`
}

// Archive moves an active entry into the archived tree. CLI archive only
// changes the repository; the TUI uninstalls first (FR-013).
func (s *Services) Archive(ctx context.Context, name string, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("archive: %w", err)
	}
	if entry == nil {
		return nil, notFound("archive", name)
	}
	newPath, err := s.Repo.Archive(ctx, entry, opts)
	if err != nil {
		return nil, err
	}
	return &LifecycleResult{Action: "archive", Name: name, DryRun: opts.DryRun, Path: newPath, Success: true}, nil
}

// Unarchive moves an archived entry back to its kind's tree.
func (s *Services) Unarchive(ctx context.Context, name string, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("unarchive: %w", err)
	}
	if entry == nil {
		return nil, notFound("unarchive", name)
	}
	newPath, err := s.Repo.Unarchive(ctx, entry, opts)
	if err != nil {
		return nil, err
	}
	return &LifecycleResult{Action: "unarchive", Name: name, DryRun: opts.DryRun, Path: newPath, Success: true}, nil
}

// Delete permanently removes an entry (requires --force).
func (s *Services) Delete(ctx context.Context, name string, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("delete: %w", err)
	}
	if entry == nil {
		return nil, notFound("delete", name)
	}
	if err := s.Repo.Delete(ctx, entry, opts); err != nil {
		return nil, err
	}
	return &LifecycleResult{Action: "delete", Name: name, DryRun: opts.DryRun, Path: entry.Path, Success: true}, nil
}

// Normalize moves a non-standard entry (found outside its expected
// provider-nested location) — or an active entry, relocating it to another
// provider — into provider's location for its kind (provider defaults to
// "local" when empty). An active entry's installs point at the old path, so
// moving it relinks them to the new location (uninstall -> move -> reinstall),
// like Convert. DryRun previews the destination without writing.
func (s *Services) Normalize(ctx context.Context, name, provider string, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}
	if entry == nil {
		return nil, notFound("normalize", name)
	}
	relink := entry.Status == common.StatusActive && !opts.DryRun
	if relink {
		// Validate the destination before touching any links, so a move to an
		// occupied location fails without leaving the entry uninstalled.
		if _, err := s.Repo.Normalize(ctx, entry, provider, engines.LifecycleOptions{DryRun: true}); err != nil {
			return nil, err
		}
		s.uninstallLinks(entry)
	}
	dest, err := s.Repo.Normalize(ctx, entry, provider, opts)
	if err != nil {
		if relink {
			// The dry-run validation only rules out an occupied destination; the
			// move itself can still fail (e.g. an uncreatable parent). Restore
			// the old links so a failed move does not leave the entry
			// uninstalled at its old location.
			s.installLinks(entry)
		}
		return nil, err
	}
	if relink {
		moved := *entry
		moved.Path = dest
		s.installLinks(&moved)
	}
	return &LifecycleResult{Action: "normalize", Name: name, DryRun: opts.DryRun, Path: dest, Success: true}, nil
}

// Convert flips a directory entry's kind, cleaning old-kind links and
// reinstalling under the new kind (FR-026).
func (s *Services) Convert(ctx context.Context, name string, targetKind common.EntryKind, opts engines.LifecycleOptions) (*LifecycleResult, error) {
	entry, err := s.ResolveEntry(name)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	if entry == nil {
		return nil, notFound("convert", name)
	}

	if opts.DryRun {
		dest, err := s.Repo.ConvertDest(entry, targetKind)
		if err != nil {
			return nil, err
		}
		return &LifecycleResult{Action: "convert", Name: name, DryRun: true, Path: dest, Type: &targetKind, Success: true}, nil
	}

	// 1. Remove old-kind managed links.
	s.uninstallLinks(entry)
	// 2. Flip marker, move tree, drop origin.
	newEntry, err := s.Repo.ConvertContent(ctx, entry, targetKind)
	if err != nil {
		return nil, err
	}
	// 3. Reinstall under the new kind.
	s.installLinks(newEntry)
	return &LifecycleResult{Action: "convert", Name: name, Path: newEntry.Path, Type: &targetKind, Success: true}, nil
}

func (s *Services) uninstallLinks(entry *common.Entry) {
	for _, t := range s.Installer.Targets(entry) {
		tx := &dal.FileTransaction{}
		if _, err := s.Installer.Uninstall(tx, entry, t); err == nil {
			tx.Commit()
		}
	}
}

func (s *Services) installLinks(entry *common.Entry) {
	for _, t := range s.Installer.Targets(entry) {
		tx := &dal.FileTransaction{}
		if _, err := s.Installer.Install(tx, entry, t, false); err == nil {
			tx.Commit()
		}
	}
}

func notFound(action, name string) error {
	return common.WithExitCode(fmt.Errorf("%s: entry %q not found", action, name), common.ExitObject)
}

func ambiguous(name string, paths []string) error {
	return common.WithExitCode(fmt.Errorf("entry %q is ambiguous; %d entries share this name:\n  %s\nre-run with one of the paths above", name, len(paths), strings.Join(paths, "\n  ")), common.ExitObject)
}
