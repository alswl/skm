// Package services is skm's orchestration layer: it composes the domain
// packages — engines (repository file mechanics), installer (target
// installation), providers (acquisition) — into the operations the CLI and
// TUI invoke, and it hosts the subprocess plugin protocols on top of
// pkg/plugins. Domain types are not re-exported here; callers that need one
// import the package that owns it.
package services
