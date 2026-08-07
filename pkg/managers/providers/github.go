package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitHostProvider clones git repositories (URLs or owner/repo shorthand) into
// a temp staging directory using the system git binary. GitHub and GitLab
// share this exact access pattern — only the id/label/host differ — so both
// resolve owner/repo shorthand to their own https host and clone over https
// (plan.md constraints: no network except via git).
type gitHostProvider struct {
	id, label, host, description string
}

// NewGitHub returns the built-in GitHub provider.
func NewGitHub() Provider {
	return gitHostProvider{
		id: "github", label: "GitHub / git repositories", host: "github.com",
		description: "Clones git URLs (git@/https/ssh/.git) and owner/repo shorthand",
	}
}

// NewGitLab returns the built-in GitLab provider — the same access pattern as
// GitHub, with gitlab.com as the shorthand host (http clone).
func NewGitLab() Provider {
	return gitHostProvider{
		id: "gitlab", label: "GitLab", host: "gitlab.com",
		description: "Clones git URLs (git@/https/ssh/.git) and owner/repo shorthand",
	}
}

// ID returns the provider id.
func (g gitHostProvider) ID() string { return g.id }

// Label returns the human label.
func (g gitHostProvider) Label() string { return g.label }

// Capability describes what this provider handles.
func (g gitHostProvider) Capability() Capability {
	return Capability{
		ID: g.id, Label: g.label,
		Description: g.description,
		Schemes:     []string{"git@", "https://", "ssh://", "owner/repo"},
	}
}

// Normalize rewrites an owner/repo shorthand to the provider's https URL; real
// URLs are returned unchanged.
func (g gitHostProvider) Normalize(address string) (string, error) {
	return normalizeGitURL(address, g.host), nil
}

// CanHandle reports whether address is a git URL or an owner/repo shorthand.
func (g gitHostProvider) CanHandle(address string) bool {
	return isGitURL(address) || isOwnerRepoShorthand(address)
}

// Fetch clones the address into a temp dir and returns its path.
func (g gitHostProvider) Fetch(ctx context.Context, address string) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-"+g.id+"-*")
	if err != nil {
		return "", fmt.Errorf("%s: create temp dir: %w", g.id, err)
	}
	url := normalizeGitURL(address, g.host)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("%s: git clone %s: %s: %w", g.id, url, strings.TrimSpace(string(out)), err)
	}
	return tmp, nil
}

// isGitURL detects common git URL forms.
func isGitURL(s string) bool {
	return strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "git://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasSuffix(s, ".git")
}

// isOwnerRepoShorthand detects "owner/repo".
func isOwnerRepoShorthand(s string) bool {
	parts := strings.Split(s, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(s, " \t")
}

// normalizeGitURL converts an owner/repo shorthand into the host's https URL
// and leaves real URLs untouched.
func normalizeGitURL(s, host string) string {
	if isOwnerRepoShorthand(s) {
		return "https://" + host + "/" + s
	}
	return s
}
