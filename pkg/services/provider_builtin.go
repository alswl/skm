package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitBackedProvider is a built-in Provider for an internal source recognized
// by its own address scheme, resolved to a git URL and fetched via the
// system git binary — the same mechanism GitHub already uses.
//
// gitlab.go and skillssh.go are built on this shared helper: gitlab uses
// "gitlab://<org>/<repo>" against gitlab.com (SKM_GITLAB_HOST); skills-sh uses
// "skills.sh://<owner>/<repo>" against github.com (SKM_SKILLS_SH_HOST), since
// skills.sh indexes skills living in public GitHub repos. Each host is
// overridable per-provider via its EnvHostVar without any interface change.
type gitBackedProvider struct {
	id, label, scheme string
	envHostVar        string
	defaultHost       string
	icon              string
}

// ID returns the provider id.
func (g gitBackedProvider) ID() string { return g.id }

// Label returns the human label.
func (g gitBackedProvider) Label() string { return g.label }

// Capability describes what this provider handles.
func (g gitBackedProvider) Capability() Capability {
	return Capability{
		ID: g.id, Label: g.label,
		Description: fmt.Sprintf("Fetches git-backed assets from %s<path> addresses", g.scheme),
		Schemes:     []string{g.scheme},
		Icon:        g.icon,
	}
}

// CanHandle reports whether address uses this provider's scheme.
func (g gitBackedProvider) CanHandle(address string) bool {
	return strings.HasPrefix(address, g.scheme)
}

// Normalize resolves a scheme-prefixed address to its canonical git URL.
// An address that doesn't use this provider's scheme is returned unchanged
// (Local/GitHub convention: normalize is best-effort, not validation).
func (g gitBackedProvider) Normalize(address string) (string, error) {
	if !g.CanHandle(address) {
		return address, nil
	}
	path := strings.TrimPrefix(address, g.scheme)
	return fmt.Sprintf("https://%s/%s.git", g.resolveHost(), path), nil
}

// resolveHost returns the configured host override, or the reconstructed
// default (research R2).
func (g gitBackedProvider) resolveHost() string {
	if v := os.Getenv(g.envHostVar); v != "" {
		return v
	}
	return g.defaultHost
}

// Fetch clones the resolved git URL into a temp dir and returns its path.
func (g gitBackedProvider) Fetch(ctx context.Context, address string) (string, error) {
	if !g.CanHandle(address) {
		return "", &ProviderError{Code: CodeUnsupportedAddress,
			Message: fmt.Sprintf("%s: address %q does not use the %s scheme", g.id, address, g.scheme)}
	}
	url, _ := g.Normalize(address)
	tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
	if err != nil {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("%s: create temp dir: %s", g.id, err)}
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", &ProviderError{Code: CodeFetchFailed,
			Message: fmt.Sprintf("%s: git clone %s: %s: %s", g.id, url, strings.TrimSpace(string(out)), err)}
	}
	return tmp, nil
}
