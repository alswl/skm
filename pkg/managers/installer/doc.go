// Package installer places and removes entries at install targets: it
// dispatches per (EntryKind, InstallStrategy) to symlinks, command markers,
// command adapters, or an out-of-process Target plugin, and derives
// InstallState for the read paths.
package installer
