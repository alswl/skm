// Package services is skm's single business layer. It owns repository
// scanning and lifecycle operations, provider resolution and plugins, target
// installation, and the cross-model operations used by both the CLI and TUI.
// It depends on dal for filesystem transactions and configuration I/O; callers
// never need an additional manager or orchestration package.
package services
