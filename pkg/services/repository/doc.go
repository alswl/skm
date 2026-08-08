// Package repository owns the on-disk entry tree: scanning, importing,
// updating, and lifecycle transitions (archive/unarchive/delete/convert) for
// skills and commands under a repository root. A single-model service
// (pkg/services/*), consumed by the pkg/managers scenario layer.
package repository
