package services

import (
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/engines"
)

// Services is the single orchestration entry shared by the CLI and TUI. All
// write paths flow through it (no two business logics invariant).
type Services struct {
	Cfg                  *config.Config
	Logger               *common.Logger
	Repo                 *engines.Repository
	Registry             *Registry
	Installer            *Installer
	TargetPlugins        map[string]TargetDriver
	TargetPluginFailures []PluginLoadFailure
}

// New wires a Services instance from resolved config. Built-in providers
// (Local, SelfBuild, GitHub, GitLab, Skills.sh) are registered first in a
// stable order (FR-035, FR-009), then subprocess plugins from the configured
// plugin dirs (US8). Target plugins are discovered the same way, so a target
// declaring a "plugin:<id>" strategy resolves against an already-loaded set.
func New(cfg *config.Config, logger *common.Logger) (*Services, error) {
	reg := NewRegistry()
	builtins := []Provider{
		NewLocal(), NewSelfBuild(), NewGitHub(),
		NewGitLab(), NewSkillsSh(),
	}
	for _, p := range builtins {
		if err := reg.Register(p); err != nil {
			return nil, err
		}
	}

	loadedTargetPlugins, targetPluginFailures := DiscoverTargetPlugins(cfg.PluginDirs, logger)
	targetPlugins := make(map[string]TargetDriver, len(loadedTargetPlugins))
	for _, p := range loadedTargetPlugins {
		targetPlugins[p.ID()] = externalTargetDriver{p}
	}
	for _, f := range targetPluginFailures {
		logger.Warn("target plugin load failed (isolated)", "path", f.Path, "err", f.Reason.Message)
	}

	svc := &Services{
		Cfg:                  cfg,
		Logger:               logger,
		Repo:                 engines.NewRepository(cfg.Root),
		Registry:             reg,
		TargetPlugins:        targetPlugins,
		TargetPluginFailures: targetPluginFailures,
		Installer:            NewInstaller(cfg.Targets, targetPlugins),
	}
	svc.loadPlugins()
	svc.logInvalidTargets()
	return svc, nil
}

// logInvalidTargets warns (stderr) about each targets.json entry that could
// not be interpreted, without blocking startup — the readable entries in
// Cfg.Targets still load (FR-016).
func (s *Services) logInvalidTargets() {
	for _, inv := range s.Cfg.InvalidTargets {
		s.Logger.Warn("invalid targets.json entry (isolated)", "reason", inv.Reason, "raw", string(inv.Raw))
	}
}

// loadPlugins registers subprocess plugins after the built-ins in a stable
// order, isolating failures and rejecting duplicate ids (FR-035). Load
// failures are retained on the registry so `provider list/validate` can
// report the specific reason (002-open-provider-target FR-006).
func (s *Services) loadPlugins() {
	loaded, failures := DiscoverPlugins(s.Cfg.PluginDirs, s.Logger)
	for _, p := range loaded {
		if err := s.Registry.Register(p); err != nil {
			s.Logger.Warn("plugin registration skipped", "id", p.ID(), "err", err.Error())
		}
	}
	s.Registry.SetLoadFailures(failures)
}

// Scan returns the current entry list.
func (s *Services) Scan() []*common.Entry {
	return s.Repo.Scan()
}

// FindEntry returns the entry addressed by ref, or nil. An entry's identity
// is its repository-relative path, so a path-like ref (contains a separator,
// or is absolute) is matched against paths first — "archived/local/demo"
// resolves to the archived copy of a same-named pair; a bare name falls back
// to the global name space for ergonomics. Error entries carry a name and are
// findable.
func (s *Services) FindEntry(ref string) *common.Entry {
	if ref == "" {
		return nil
	}
	want := filepath.Clean(ref)
	if filepath.IsAbs(want) {
		want = s.Repo.RelPath(want)
	}
	entries := s.Repo.Scan()
	if strings.ContainsAny(ref, `/\`) {
		for _, e := range entries {
			if s.Repo.RelPath(e.Path) == want {
				return e
			}
		}
	}
	for _, e := range entries {
		if e.Name == ref {
			return e
		}
	}
	return nil
}
