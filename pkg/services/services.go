package services

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/managers/installer"
	"github.com/alswl/skm/skm/pkg/managers/providers"
	"github.com/alswl/skm/skm/pkg/managers/repository"
)

// Services is the single orchestration entry shared by the CLI and TUI. All
// write paths flow through it (no two business logics invariant).
type Services struct {
	Cfg       *config.Config
	Logger    *common.Logger
	Repo      *repository.Repository
	Registry  *providers.Registry
	Installer *installer.Installer
}

// New wires a Services instance from resolved config. Built-in providers
// (Local, GitHub) are registered first in a stable order (FR-035), then
// subprocess plugins from the configured plugin dirs (US8).
func New(cfg *config.Config, logger *common.Logger) (*Services, error) {
	reg := providers.NewRegistry()
	if err := reg.Register(providers.NewLocal()); err != nil {
		return nil, err
	}
	if err := reg.Register(providers.NewGitHub()); err != nil {
		return nil, err
	}
	svc := &Services{
		Cfg:       cfg,
		Logger:    logger,
		Repo:      repository.New(cfg.Root),
		Registry:  reg,
		Installer: installer.New(cfg.Targets),
	}
	svc.loadPlugins()
	return svc, nil
}

// loadPlugins registers subprocess plugins after the built-ins in a stable
// order, isolating failures and rejecting duplicate ids (FR-035).
func (s *Services) loadPlugins() {
	for _, p := range providers.DiscoverPlugins(s.Cfg.PluginDirs, s.Logger) {
		if err := s.Registry.Register(p); err != nil {
			s.Logger.Warn("plugin registration skipped", "id", p.ID(), "err", err.Error())
		}
	}
}

// Scan returns the current entry list.
func (s *Services) Scan() []*common.Entry {
	return s.Repo.Scan()
}

// FindEntry returns the entry with the given name across the repository, or
// nil. Error entries carry a name (on-disk identity) and are findable.
func (s *Services) FindEntry(name string) *common.Entry {
	for _, e := range s.Repo.Scan() {
		if e.Name == name {
			return e
		}
	}
	return nil
}
