// Package services is a namespace, not a code-bearing package: it groups the
// three single-model services — installer (links/adapters/install-state),
// providers (registry + fetch), and repository (scan/import/update/lifecycle/
// convert/discover) — as a cohesive unit distinct from pkg/managers
// (scenario orchestration, calling multiple services and/or dal) and pkg/dal
// (I/O). Nothing is defined directly in this package; import its
// subpackages. Intentionally test-free: no logic, nothing to test.
// (003-engineering-optimization R3/FR-020 — this package previously lived at
// pkg/managers; the two names were swapped to match granularity: manager =
// larger/scenario, service = smaller/single-model.)
package services
