package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

type deploySource struct {
	path, provider, group, address, clone string
	cleanup                               func()
}

func (d *deploySource) origin(path string) *common.Origin {
	id := d.provider
	origin := &common.Origin{Address: d.address, ProviderID: &id}
	if id == "local" {
		origin.Address = path
	} else {
		rel, _ := filepath.Rel(d.path, path)
		if rel != "." {
			origin.Subpath = filepath.ToSlash(rel)
		}
	}
	return origin
}

func (s *Services) acquireDeploySource(ctx context.Context, opts DeployOptions) (*deploySource, error) {
	address := normalizeImportSource(opts.Repo)
	if _, err := os.Lstat(address); err == nil {
		path, err := filepath.EvalSymlinks(address)
		if err != nil {
			return nil, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("deploy: source must be a directory")
		}
		if dal.IsDir(filepath.Join(path, "objects")) && dal.IsDir(filepath.Join(path, "refs")) && dal.PathExists(filepath.Join(path, "HEAD")) {
			return nil, fmt.Errorf("deploy: bare Git repositories are unsupported; provide a working directory")
		}
		return &deploySource{path: path, provider: "local", address: path, clone: "direct", cleanup: func() {}}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Explicit filesystem paths must never fall through to a network provider.
	if filepath.IsAbs(address) || strings.HasPrefix(address, ".") || strings.HasPrefix(address, "~") {
		return nil, fmt.Errorf("deploy: local source %q does not exist", address)
	}
	if opts.DryRun {
		return nil, fmt.Errorf("deploy: --dry-run requires a local directory; remote acquisition would write temporary files")
	}
	p := s.Registry.Match(address)
	if p == nil {
		return nil, fmt.Errorf("deploy: no provider for %q", address)
	}
	path, id, group, origin, cleanup, err := s.fetchProvider(ctx, p, address)
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err == nil {
		canonical, err = filepath.Abs(canonical)
	}
	if err != nil {
		cleanup()
		return nil, err
	}
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." || strings.ContainsAny(id, `\/`) {
		cleanup()
		return nil, fmt.Errorf("deploy: unsafe provider id %q", id)
	}
	if group != "" && !filepath.IsLocal(group) {
		cleanup()
		return nil, fmt.Errorf("deploy: unsafe provider group %q", group)
	}
	return &deploySource{path: canonical, provider: id, group: group, address: origin.Address, clone: "cloned", cleanup: cleanup}, nil
}

func discoverDeploySkills(root string, only []string) ([]*common.Entry, error) {
	var all []*common.Entry
	names := map[string]bool{}
	repo := engines.NewRepository(root)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && d.IsDir() && (strings.HasPrefix(d.Name(), ".") || (filepath.Dir(path) == root && (d.Name() == "archived" || d.Name() == "commands"))) {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("deploy: source traversal contains symlink %q", path)
			}
			return nil
		}
		marker := filepath.Join(path, "SKILL.md")
		if _, err := os.Lstat(marker); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		if err := engines.ValidateSkillTree(path); err != nil {
			return err
		}
		kind, name, err := repo.ProbeStaged(path)
		if err != nil {
			return err
		}
		if kind != common.KindSkill {
			return fmt.Errorf("deploy: %q is not a skill", path)
		}
		if err = engines.ValidateSkillName(name); err != nil {
			return err
		}
		if names[name] {
			return common.WithExitCode(fmt.Errorf("deploy: duplicate skill name %q", name), common.ExitObject)
		}
		names[name] = true
		all = append(all, &common.Entry{Name: name, Kind: kind, Path: path, Status: common.StatusActive})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, common.WithExitCode(err, common.ExitObject)
	}
	if len(all) == 0 {
		return nil, common.WithExitCode(fmt.Errorf("deploy: no skills found"), common.ExitObject)
	}
	if len(only) == 0 {
		return all, nil
	}
	selected := map[string]bool{}
	for _, name := range only {
		if !names[name] {
			return nil, fmt.Errorf("deploy: --only %q does not match a skill", name)
		}
		selected[name] = true
	}
	var out []*common.Entry
	for _, e := range all {
		if selected[e.Name] {
			out = append(out, e)
		}
	}
	return out, nil
}

// canonicalDeployPath resolves existing ancestors without creating missing ones.
func canonicalDeployPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(abs); err == nil {
		return filepath.EvalSymlinks(abs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := canonicalDeployPath(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(parent); err == nil && !info.IsDir() {
		return "", fmt.Errorf("deploy: parent %q is not a directory", parent)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func newDeployDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(abs); err == nil {
		return "", common.WithExitCode(fmt.Errorf("deploy: destination %q already exists; choose a new directory", abs), common.ExitObject)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("deploy: destination parent must be a directory")
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func pathsOverlap(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return true
	}
	rel, err = filepath.Rel(b, a)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

func (s *Services) deployTargets(names []string) ([]common.InstallTarget, error) {
	var out []common.InstallTarget
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		t, ok := s.Installer.TargetByName(name)
		if !ok {
			return nil, fmt.Errorf("deploy: unknown target %q", name)
		}
		strategy, ok := t.EffectiveStrategy(common.KindSkill)
		if !t.AcceptsKind(common.KindSkill) || !ok || strategy != common.StrategySkillSymlink {
			return nil, fmt.Errorf("deploy: target %q requires built-in skill-symlink strategy", name)
		}
		if t.Path == "" {
			return nil, fmt.Errorf("deploy: target %q has no path", name)
		}
		path, err := canonicalDeployPath(t.Path)
		if err != nil {
			return nil, err
		}
		t.Path = path
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return nil, fmt.Errorf("deploy: target %q is not a directory", name)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, prev := range out {
			if pathsOverlap(prev.Path, t.Path) {
				return nil, fmt.Errorf("deploy: targets %q and %q overlap", prev.Name, t.Name)
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func preflightDeploy(source, destination string, skills []*common.Entry, targets []common.InstallTarget, force bool) error {
	if destination != "" && pathsOverlap(source, destination) {
		return fmt.Errorf("deploy: source and repository overlap")
	}
	for _, t := range targets {
		if pathsOverlap(source, t.Path) || (destination != "" && pathsOverlap(destination, t.Path)) {
			return fmt.Errorf("deploy: target %q overlaps source or repository", t.Name)
		}
		for _, e := range skills {
			if t.NameRule != "" && !common.NameSatisfies(t.NameRule, e.Name) {
				return fmt.Errorf("deploy: skill %q violates target %q name rule %q", e.Name, t.Name, t.NameRule)
			}
			slot := filepath.Join(t.Path, e.Name)
			if _, err := os.Lstat(slot); err == nil && !force {
				return common.WithExitCode(fmt.Errorf("deploy: slot %q exists; use --force to replace this target slot", slot), common.ExitObject)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
