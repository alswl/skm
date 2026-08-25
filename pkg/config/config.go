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
	// ConfigDir holds targets.json; defaults to ~/.config/skm.
	ConfigDir string
	// Targets is the install-target list (from targets.json or built-ins).
	Targets []common.InstallTarget
	// InvalidTargets are targets.json entries that could not be interpreted,
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
	ConfigDirName   = ".config/skm"
	EnvPluginsDir   = "SKM_PLUGINS_DIR"
	targetsFileName = "targets.json"
	// EnvXDGConfigHome is the XDG Base Directory config-home override
	// (https://specifications.freedesktop.org/basedir-spec/latest/). Read
	// directly rather than via os.UserConfigDir(), which on macOS always
	// returns ~/Library/Application Support regardless of this variable and
	// would silently break the documented ~/.config/skm default.
	EnvXDGConfigHome = "XDG_CONFIG_HOME"
	// LegacyConfigDirName is the original skmgr config directory
	// (docs/req.md: `--config` 默认 `~/.config/skill-manager`). It is
	// consulted only as a fallback when the caller didn't pass --config and
	// the new-style ~/.config/skm/targets.json doesn't exist, so an upgrading
	// user's existing config loads without manual migration
	// (002-open-provider-target FR-015, research R6).
	LegacyConfigDirName = ".config/skill-manager"
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

// DefaultLegacyConfigDir returns the original skmgr config directory
// (~/.config/skill-manager), falling back to the literal value if HOME is
// unset.
func DefaultLegacyConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/skill-manager"
	}
	return filepath.Join(home, LegacyConfigDirName)
}

// DefaultPluginDirs returns the default plugin directory: "plugins" under
// DefaultConfigDir(), so it follows the same $XDG_CONFIG_HOME override.
func DefaultPluginDirs() []string {
	return []string{filepath.Join(DefaultConfigDir(), "plugins")}
}

// DefaultTargets returns the built-in targets restored when targets.json is
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
	explicit := configDir != ""
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	settings := newSettings(configDir)
	root, err := DiscoverRoot(rootFrom(settings, rootFlag))
	if err != nil {
		return nil, err
	}
	resolvedDir, targets, invalid := loadTargetsWithLegacyFallback(configDir, explicit)
	plugins := pluginDirsFrom(settings)
	return &Config{
		Root:           root,
		ConfigDir:      resolvedDir,
		Targets:        targets,
		InvalidTargets: invalid,
		PluginDirs:     plugins,
	}, nil
}

// LoadForDeploy builds a Config without requiring a repository root. The
// deploy command operates on its --repo source (which may not exist yet on the
// target machine), so no local repository is needed.
func LoadForDeploy(configDir string) *Config {
	explicit := configDir != ""
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	resolvedDir, targets, invalid := loadTargetsWithLegacyFallback(configDir, explicit)
	return &Config{
		ConfigDir:      resolvedDir,
		Targets:        targets,
		InvalidTargets: invalid,
		PluginDirs:     pluginDirsFrom(newSettings(configDir)),
	}
}

// loadTargetsWithLegacyFallback loads configDir/targets.json; when the
// caller didn't pass --config (explicit is false) and no file exists there,
// it falls back to the legacy skmgr config directory before restoring
// built-in defaults, so an upgrading user's existing config is found without
// manual migration (FR-015). The resolved directory is returned too, so
// Config.ConfigDir points at wherever the targets actually came from — a
// subsequent target add/update/remove writes back to that same file instead
// of silently forking into a fresh new-style config.
func loadTargetsWithLegacyFallback(configDir string, explicit bool) (resolvedDir string, valid []common.InstallTarget, invalid []InvalidTarget) {
	if explicit {
		valid, invalid = loadTargets(configDir)
		return configDir, valid, invalid
	}
	if _, err := os.Stat(filepath.Join(configDir, targetsFileName)); err == nil {
		valid, invalid = loadTargets(configDir)
		return configDir, valid, invalid
	}
	legacyDir := DefaultLegacyConfigDir()
	if _, err := os.Stat(filepath.Join(legacyDir, targetsFileName)); err != nil {
		valid, invalid = loadTargets(configDir) // neither exists: defaults
		return configDir, valid, invalid
	}
	valid, invalid = loadTargets(legacyDir)
	return legacyDir, valid, invalid
}

// loadTargets reads <configDir>/targets.json, migrating v1 (legacy Kind-only)
// entries and validating v2 entries independently (research R6): a
// per-entry problem is reported in invalid, not a fallback to defaults, and
// readable entries still load (FR-016). Built-in defaults are restored only
// when the file is missing or has zero interpretable entries (FR-003);
// otherwise the built-ins are always merged in alongside the user's entries
// (mergeWithBuiltins) so a custom target never hides them.
func loadTargets(configDir string) (valid []common.InstallTarget, invalid []InvalidTarget) {
	data, err := os.ReadFile(filepath.Join(configDir, targetsFileName))
	if err != nil {
		return DefaultTargets(), nil
	}
	valid, invalid, err = ParseTargets(data)
	if err != nil {
		// The document itself isn't a JSON array: nothing to report
		// per-entry: restore defaults.
		return DefaultTargets(), nil
	}
	if len(valid) == 0 {
		return DefaultTargets(), invalid
	}
	return mergeWithBuiltins(valid), invalid
}

// mergeWithBuiltins combines the always-present built-in targets with the
// user's targets.json entries: a user entry sharing a built-in's name
// overrides it (in the built-in's original position); every other user entry
// is appended after, in file order. This keeps the four built-ins visible in
// `target list` and the install flow even once the user has added targets of
// their own (a targets.json with ≥1 entry used to replace the built-ins
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
