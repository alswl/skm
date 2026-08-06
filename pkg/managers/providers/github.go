package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitHub clones git repositories (URLs or owner/repo shorthand) into a temp
// staging directory using the system git binary (plan.md constraints: no
// network except via git).
type GitHub struct{}

// NewGitHub returns the built-in GitHub provider.
func NewGitHub() *GitHub { return &GitHub{} }

// ID returns the provider id.
func (GitHub) ID() string { return "github" }

// Label returns the human label.
func (GitHub) Label() string { return "GitHub / git repositories" }

// CanHandle reports whether address is a git URL or an owner/repo shorthand.
func (GitHub) CanHandle(address string) bool {
	return isGitURL(address) || isOwnerRepoShorthand(address)
}

// Fetch clones the address into a temp dir and returns its path.
func (g GitHub) Fetch(ctx context.Context, address string) (string, error) {
	tmp, err := os.MkdirTemp("", "skm-gh-*")
	if err != nil {
		return "", fmt.Errorf("github: create temp dir: %w", err)
	}
	url := normalizeGitURL(address)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, tmp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", fmt.Errorf("github: git clone %s: %s: %w", url, strings.TrimSpace(string(out)), err)
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

// normalizeGitURL converts an owner/repo shorthand into an https URL and
// leaves real URLs untouched.
func normalizeGitURL(s string) string {
	if isOwnerRepoShorthand(s) {
		return "https://github.com/" + s
	}
	return s
}
