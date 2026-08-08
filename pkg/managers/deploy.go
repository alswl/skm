package managers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/services/repository"
)

// DeployOptions controls a deploy action.
type DeployOptions struct {
	Repo    string
	Targets []string
	Only    []string // --only entry names
	Force   bool
	DryRun  bool
	// CacheDir overrides the persistent clone destination (test hook).
	CacheDir string
}

// DeployResult is the CLI JSON report for deploy (contract/cli-json.md).
type DeployResult struct {
	Repo    string                 `json:"repo"`
	Targets []string               `json:"targets"`
	Skills  []string               `json:"skills"`
	Clone   string                 `json:"clone"`
	Pull    *string                `json:"pull"`
	Results []common.InstallReport `json:"results"`
	Success bool                   `json:"success"`
}

// Deploy routes a git URL/bare repo to clone-or-ff-only-pull, a local git repo
// to ff-only pull, and a plain directory to direct use, then scans and batch
// installs (FR-029). It refuses an existing clone with an inconsistent remote
// or a non-git non-empty clone target.
func (s *Services) Deploy(ctx context.Context, opts DeployOptions) (*DeployResult, error) {
	targetsOut := opts.Targets
	if targetsOut == nil {
		targetsOut = []string{}
	}
	result := &DeployResult{
		Repo:    opts.Repo,
		Targets: targetsOut,
		Skills:  []string{},
		Results: []common.InstallReport{},
	}

	src, mode, err := s.prepareRepo(ctx, opts)
	if err != nil {
		return nil, err
	}
	switch mode {
	case "cloned":
		result.Clone = "cloned"
	case "pulled":
		p := "ff-only"
		result.Clone = "pulled"
		result.Pull = &p
	case "direct":
		result.Clone = "direct"
	}

	entries := repository.New(src).Scan()
	selected := filterActiveAndOnly(entries, opts.Only)
	targets, err := s.resolveDeployTargets(selected, opts.Targets)
	if err != nil {
		return nil, err
	}
	for _, e := range selected {
		result.Skills = append(result.Skills, e.Name)
		for _, t := range targets {
			if !s.Installer.Matches(e, t) {
				continue
			}
			if opts.DryRun {
				result.Results = append(result.Results, common.InstallReport{Target: t.Name, Status: common.InstallInstalled, Changed: true})
				continue
			}
			tx := &dal.FileTransaction{}
			changed, err := s.Installer.Install(tx, e, t, opts.Force)
			if err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			tx.Commit()
			result.Results = append(result.Results, common.InstallReport{Target: t.Name, Status: common.InstallInstalled, Changed: changed})
		}
	}
	result.Success = true
	return result, nil
}

// prepareRepo materializes the deploy source and returns its local path plus
// how it was obtained.
func (s *Services) prepareRepo(ctx context.Context, opts DeployOptions) (string, string, error) {
	repo := opts.Repo
	cacheBase := opts.CacheDir
	if cacheBase == "" {
		cacheBase = defaultDeployCache()
	}
	if isGitURL(repo) || (dal.PathExists(repo) && isBareRepo(repo)) {
		dir := filepath.Join(cacheBase, sanitizeRepoName(repo))
		mode, err := s.cloneOrPull(ctx, repo, dir)
		if err != nil {
			return "", "", err
		}
		return dir, mode, nil
	}
	if !dal.PathExists(repo) {
		return "", "", common.WithExitCode(fmt.Errorf("deploy: source %q does not exist", repo), common.ExitError)
	}
	if dal.IsDir(filepath.Join(repo, ".git")) {
		if err := gitPullFFOnly(ctx, repo); err != nil {
			return "", "", common.WithExitCode(err, common.ExitError)
		}
		return repo, "pulled", nil
	}
	return repo, "direct", nil
}

// isBareRepo reports whether dir is a bare git repository (HEAD + objects +
// refs, no worktree).
func isBareRepo(dir string) bool {
	return dal.IsDir(filepath.Join(dir, "objects")) &&
		dal.IsDir(filepath.Join(dir, "refs")) &&
		dal.PathExists(filepath.Join(dir, "HEAD"))
}

// cloneOrPull clones a URL into dir, or ff-only pulls when an existing clone
// matches. It refuses an inconsistent remote or a non-git non-empty dir.
func (s *Services) cloneOrPull(ctx context.Context, url, dir string) (string, error) {
	if dal.PathExists(dir) {
		if !dal.IsDir(filepath.Join(dir, ".git")) {
			// Non-git non-empty clone target -> refuse.
			return "", common.WithExitCode(fmt.Errorf("deploy: clone target %s exists but is not a git repo", dir), common.ExitError)
		}
		if remote := gitRemoteURL(dir); remote != "" && remote != normalizeGitURL(url) {
			return "", common.WithExitCode(fmt.Errorf("deploy: existing clone %s has remote %q, expected %q", dir, remote, normalizeGitURL(url)), common.ExitError)
		}
		if err := gitPullFFOnly(ctx, dir); err != nil {
			return "", common.WithExitCode(err, common.ExitError)
		}
		return "pulled", nil
	}
	if err := gitClone(ctx, url, dir); err != nil {
		return "", common.WithExitCode(err, common.ExitError)
	}
	return "cloned", nil
}

func filterActiveAndOnly(entries []*common.Entry, only []string) []*common.Entry {
	set := map[string]bool{}
	for _, n := range only {
		set[n] = true
	}
	var out []*common.Entry
	for _, e := range entries {
		if e.Status != common.StatusActive {
			continue
		}
		if len(only) > 0 && !set[e.Name] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// resolveDeployTargets resolves the requested target names (or all targets
// when empty).
func (s *Services) resolveDeployTargets(entries []*common.Entry, names []string) ([]common.InstallTarget, error) {
	if len(names) == 0 {
		return s.Cfg.Targets, nil
	}
	var out []common.InstallTarget
	for _, n := range names {
		t, ok := s.Installer.TargetByName(n)
		if !ok {
			return nil, common.WithExitCode(fmt.Errorf("deploy: unknown target %q", n), common.ExitError)
		}
		out = append(out, t)
	}
	return out, nil
}

// --- git helpers ---

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "git://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasSuffix(s, ".git")
}

func normalizeGitURL(s string) string {
	parts := strings.Split(s, "/")
	if len(parts) == 2 && !strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, "http") && !strings.HasPrefix(s, "git@") && !strings.HasPrefix(s, "git:") {
		return "https://github.com/" + s
	}
	return s
}

func gitClone(ctx context.Context, url, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", normalizeGitURL(url), dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deploy: git clone: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func gitPullFFOnly(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deploy: git pull --ff-only: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func gitRemoteURL(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultDeployCache() string {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "skm", "deploy")
	}
	return filepath.Join(os.TempDir(), "skm-deploy")
}

func sanitizeRepoName(repo string) string {
	base := repo
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		base = repo[i+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	base = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == ' ' {
			return '-'
		}
		return r
	}, base)
	if base == "" {
		return "repo"
	}
	return base
}
