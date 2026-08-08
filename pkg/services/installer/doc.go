// Package installer places and removes entries at install targets: it
// dispatches per (EntryKind, InstallStrategy) to symlinks, command markers,
// command adapters, or an out-of-process Target plugin, and derives
// InstallState for the read paths. A single-model service (pkg/services/*),
// consumed by the pkg/managers scenario layer.
package installer
