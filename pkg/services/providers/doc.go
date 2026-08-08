// Package providers resolves and fetches remote/local entry sources: the
// Provider interface, the Registry that holds them in resolution order, and
// the built-in providers (Local, SelfBuild, GitHub, GitLab, Skills.sh) — the
// subprocess plugin transport itself lives in pkg/plugins. A single-model
// service (pkg/services/*), consumed by the pkg/managers scenario layer.
package providers
