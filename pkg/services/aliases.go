package services

import "github.com/alswl/skm/skm/pkg/engines"

// Types and constructors moved to pkg/engines are re-exported here so the
// public services API (commands/, tui/) stays unchanged.

// LifecycleOptions controls archive/unarchive/delete/convert.
type LifecycleOptions = engines.LifecycleOptions

// DiscoveredSkill is an external unmanaged skill found in an install target.
type DiscoveredSkill = engines.DiscoveredSkill

// UpdateResult reports a single-entry update.
type UpdateResult = engines.UpdateResult

// DanglingInstall is a target-side installation that has no usable source.
// It is deliberately path-based: an orphan has no Entry to address it by.
type DanglingInstall = engines.DanglingInstall

// NewRepository returns a file-mechanics Repository over root.
func NewRepository(root string) *engines.Repository { return engines.NewRepository(root) }

// InitializeRepository creates the smallest safe skills repository.
func InitializeRepository(path string) (string, error) { return engines.InitializeRepository(path) }
