package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/targets"
)

// Config holds resolved runtime configuration shared by CLI and TUI.
type Config struct {
	// Root is the absolute repository root (normalized from --root or
	// discovered upward from cwd).
	Root string
	// ConfigDir holds config.yaml; defaults to ~/.config/skm.
	ConfigDir string
	// Targets is the install-target list (from config.yaml or built-ins).
	Targets []common.InstallTarget
	// InvalidTargets are config.yaml target entries that could not be interpreted,
	// each with its own reason (002-open-provider-target FR-016) — the
	// interpretable entries in Targets still load.
	InvalidTargets []InvalidTarget
	// PluginDirs are the base plugin directories scanned at startup. Each
	// contains a "providers" subdir (Provider plugins) and a "targets"
	// subdir (Target plugins), scanned separately by their own Discover*.
	PluginDirs []string
}

// Paths used when nothing overrides them. These are literal XDG-style paths
// per the observable contract in docs/req.md (see plan.md Complexity
// Tracking: intentionally not os.UserConfigDir()).
const (
	ConfigDirName = ".config/skm"
	EnvPluginsDir = "SKM_PLUGINS_DIR"
	// EnvXDGConfigHome is the XDG Base Directory config-home override
	// (https://specifications.freedesktop.org/basedir-spec/latest/). Read
	// directly rather than via os.UserConfigDir(), which on macOS always
	// returns ~/Library/Application Support regardless of this variable and
	// would silently break the documented ~/.config/skm default.
	EnvXDGConfigHome = "XDG_CONFIG_HOME"
)

// DefaultConfigDir returns the default config directory: $XDG_CONFIG_HOME/skm
// when that variable is set (XDG Base Directory spec), otherwise
// ~/.config/skm — which is also the spec's own default for XDG_CONFIG_HOME,
// so an unset environment keeps today's path unchanged.
func DefaultConfigDir() string {
	if xdg := os.Getenv(EnvXDGConfigHome); xdg != "" {
		return filepath.Join(xdg, "skm")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/skm"
	}
	return filepath.Join(home, ConfigDirName)
}

// DefaultPluginDirs returns the default plugin directory: "plugins" under
// DefaultConfigDir(), so it follows the same $XDG_CONFIG_HOME override.
func DefaultPluginDirs() []string {
	return []string{filepath.Join(DefaultConfigDir(), "plugins")}
}

// DefaultTargets returns the built-in targets restored when config.yaml has no
// missing or has zero interpretable entries (FR-003). It is also what the
// services layer diffs stored paths against to report divergence in
// `target list` and `target validate` (006-deepseek-harness-target FR-004).
func DefaultTargets() []common.InstallTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	return targets.Builtins(targets.Context{Home: home, Getenv: os.Getenv})
}

// Load builds a Config. configDir defaults to ~/.config/skm when
// empty; root is normalized/discovered by DiscoverRoot.
func Load(rootFlag, configDir string) (*Config, error) {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	set, err := loadSettings(configDir)
	if err != nil {
		return nil, err
	}
	root, err := DiscoverRoot(rootFrom(set, rootFlag))
	if err != nil {
		return nil, err
	}
	targets, invalid := loadConfiguredTargets(set.Targets)
	plugins := pluginDirsFrom(set)
	return &Config{
		Root:           root,
		ConfigDir:      configDir,
		Targets:        targets,
		InvalidTargets: invalid,
		PluginDirs:     plugins,
	}, nil
}

// LoadForDeploy builds a Config without requiring a repository root. The
// deploy command operates on its --repo source (which may not exist yet on the
// target machine), so no local repository is needed.
func LoadForDeploy(configDir string) (*Config, error) {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	set, err := loadSettings(configDir)
	if err != nil {
		return nil, err
	}
	targets, invalid := loadConfiguredTargets(set.Targets)
	return &Config{
		ConfigDir:      configDir,
		Targets:        targets,
		InvalidTargets: invalid,
		PluginDirs:     pluginDirsFrom(set),
	}, nil
}

// loadConfiguredTargets validates config.yaml's targets field. Built-ins are
// restored when the field is absent or has no valid entries.
func loadConfiguredTargets(raw []common.InstallTarget) (valid []common.InstallTarget, invalid []InvalidTarget) {
	valid, invalid = normalizeTargets(raw)
	if len(valid) == 0 {
		return DefaultTargets(), invalid
	}
	return mergeWithBuiltins(valid), invalid
}

func normalizeTargets(raw []common.InstallTarget) (valid []common.InstallTarget, invalid []InvalidTarget) {
	for _, t := range raw {
		t = expandTarget(t)
		if reason := ValidateTarget(t); reason != "" {
			invalid = append(invalid, InvalidTarget{Reason: reason})
			continue
		}
		valid = append(valid, t)
	}
	return valid, invalid
}

// mergeWithBuiltins combines the always-present built-in targets with the
// user's config.yaml targets: a user entry sharing a built-in's name
// overrides it (in the built-in's original position); every other user entry
// is appended after, in file order. This keeps the four built-ins visible in
// `target list` and the install flow even once the user has added targets of
// their own (a target list with ≥1 entry used to replace the built-ins
// outright).
func mergeWithBuiltins(userEntries []common.InstallTarget) []common.InstallTarget {
	overrides := make(map[string]common.InstallTarget, len(userEntries))
	for _, u := range userEntries {
		overrides[u.Name] = u
	}
	defaults := DefaultTargets()
	merged := make([]common.InstallTarget, 0, len(defaults)+len(userEntries))
	seen := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		if u, ok := overrides[d.Name]; ok {
			merged = append(merged, u)
		} else {
			merged = append(merged, d)
		}
		seen[d.Name] = true
	}
	for _, u := range userEntries {
		if !seen[u.Name] {
			merged = append(merged, u)
		}
	}
	return merged
}

func expandTarget(t common.InstallTarget) common.InstallTarget {
	t.Path = expandHome(t.Path)
	return t
}

// expandHome resolves a leading "~/" against the user's home directory. A path
// without the prefix, or an unresolvable home, is returned unchanged.
func expandHome(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/"))
}
