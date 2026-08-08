// Package managers is a namespace, not a code-bearing package: it groups the
// three stateful domain managers — installer (links/adapters/install-state),
// providers (registry + fetch), and repository (scan/import/update/lifecycle/
// convert/discover) — as a cohesive unit distinct from pkg/services
// (orchestration) and pkg/dal (I/O). Nothing is defined directly in this
// package; import its subpackages (003-engineering-optimization F-08).
// Intentionally test-free: no logic, nothing to test.
package managers
