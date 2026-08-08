// Installer code places and removes entries at install targets, dispatching
// per (EntryKind, InstallStrategy) to symlinks, command markers, command
// adapters, or an out-of-process Target plugin.
package services
