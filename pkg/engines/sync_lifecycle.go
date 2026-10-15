package engines

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// SyncLifecycle requires the shared repository lock held by services.
func (r *Repository) SyncLifecycle(e *common.Entry, action, provider string, newKind common.EntryKind, opts LifecycleOptions) (string, error) {
	rec, rel, err := r.SyncRecord(e)
	if err != nil {
		return "", err
	}
	if err = r.noAttempt(rel); err != nil {
		return "", err
	}
	if (action == "delete" || action == "convert") && rec.Active == "fork" {
		return "", fmt.Errorf("active fork exists at %s; run skm fork %s --upstream before %s; the retained directory will be preserved", rec.ForkPath, e.Name, action)
	}
	logical := *e
	logical.Path = filepath.Join(r.root, rel)
	var dest string
	switch action {
	case "archive":
		if e.Status != common.StatusActive {
			return "", fmt.Errorf("only active entries can be archived")
		}
		dest = r.archivedPath(&logical)
	case "unarchive":
		if e.Status != common.StatusArchived {
			return "", fmt.Errorf("only archived entries can be restored")
		}
		rest := strings.TrimPrefix(rel, "archived"+string(filepath.Separator))
		dest = filepath.Join(r.root, e.Kind.TopDir(), rest)
	case "normalize":
		if provider == "" {
			provider = "local"
		}
		if filepath.Base(provider) != provider || provider == "." || provider == ".." {
			return "", fmt.Errorf("invalid provider path")
		}
		dest = r.normalizeDest(&logical, provider)
	case "convert":
		if e.Status != common.StatusActive || newKind == e.Kind || newKind.MarkerFile() == "" {
			return "", fmt.Errorf("invalid kind conversion")
		}
		dest, err = r.ConvertDest(&logical, newKind)
		if err != nil {
			return "", err
		}
	case "delete":
		if !opts.Force {
			return "", fmt.Errorf("delete requires --force")
		}
	default:
		return "", fmt.Errorf("unsupported lifecycle action")
	}
	if dest != "" {
		if _, err = os.Lstat(dest); err == nil {
			return "", fmt.Errorf("destination already exists: %s", dest)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if opts.DryRun {
		return dest, nil
	}
	state, stateHash, err := r.recordStage(rel)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(state) }()
	contentHash, err := dal.TreeFingerprint(logical.Path)
	if err != nil {
		return "", err
	}
	moves := []dal.SyncReplacement{{Path: rel, Expected: contentHash}, {Path: dal.SyncStateRel(rel), Expected: stateHash}}
	if dest != "" {
		newRel, err := filepath.Rel(r.root, dest)
		if err != nil {
			return "", err
		}
		tmp, err := os.MkdirTemp("", "skm-lifecycle-*")
		if err != nil {
			return "", err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		if err = dal.CopySyncTree(logical.Path, tmp); err != nil {
			return "", err
		}
		if action == "convert" {
			if err = os.Rename(filepath.Join(tmp, e.Kind.MarkerFile()), filepath.Join(tmp, newKind.MarkerFile())); err != nil {
				return "", err
			}
		} else {
			rec.EntryPath = newRel
			rec.Origin.Path = newRel
			rec.Generation++
			if rec.ForkPath != "" {
				oldFork := rec.ForkPath
				rec.ForkPath = dal.SyncForkRel(newRel)
				hash, err := dal.TreeFingerprint(filepath.Join(r.root, oldFork))
				if err != nil {
					return "", err
				}
				moves = append(moves, dal.SyncReplacement{Path: oldFork, Expected: hash}, dal.SyncReplacement{Path: rec.ForkPath, Source: filepath.Join(r.root, oldFork), Expected: "absent"})
			}
			if err = dal.WriteSyncJSON(filepath.Join(state, "record.json"), rec); err != nil {
				return "", err
			}
			moves = append(moves, dal.SyncReplacement{Path: dal.SyncStateRel(newRel), Source: state, Expected: "absent"})
		}
		moves = append(moves, dal.SyncReplacement{Path: newRel, Source: tmp, Expected: "absent"})
	}
	if err = dal.CommitSync(r.root, rel, moves); err != nil {
		return "", err
	}
	return dest, nil
}
