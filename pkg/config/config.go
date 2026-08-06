package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
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
	// PluginDirs are provider plugin directories scanned at startup.
	PluginDirs []string
}

// Paths used when nothing overrides them. These are literal XDG-style paths
// per the observable contract in docs/req.md (see plan.md Complexity
// Tracking: intentionally not os.UserConfigDir()).
const (
	ConfigDirName   = ".config/skm"
	PluginDirName   = ".config/skm/plugins"
	EnvPluginsDir   = "SKM_PLUGINS_DIR"
	targetsFileName = "targets.json"
)

// DefaultConfigDir returns the default config directory
// (~/.config/skm), falling back to the literal value if HOME is
// unset.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/skm"
	}
	return filepath.Join(home, ConfigDirName)
}

// DefaultPluginDirs returns the default plugin directory
// (~/.config/skm/plugins).
func DefaultPluginDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{"~/.config/skm/plugins"}
	}
	return []string{filepath.Join(home, PluginDirName)}
}

// defaultTargets returns the built-in targets restored when targets.json is
// missing or invalid (FR-003). Codex/CodeFuse receive only skills; Claude has
// distinct skill and command targets.
func defaultTargets() []common.InstallTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	return []common.InstallTarget{
		{Name: "claude-skills", Path: filepath.Join(home, ".claude", "skills"), Builtin: true, Kind: common.KindSkill},
		{Name: "claude-commands", Path: filepath.Join(home, ".claude", "commands"), Builtin: true, Kind: common.KindCommand},
		{Name: "codex", Path: filepath.Join(home, ".codex", "skills"), Builtin: true, Kind: common.KindSkill},
		{Name: "codefuse", Path: filepath.Join(home, ".codefuse", "skills"), Builtin: true, Kind: common.KindSkill},
	}
}

// Load builds a Config. configDir defaults to ~/.config/skm when
// empty; root is normalized/discovered by DiscoverRoot.
func Load(rootFlag, configDir string) (*Config, error) {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	root, err := DiscoverRoot(rootFlag)
	if err != nil {
		return nil, err
	}
	targets := loadTargets(configDir)
	plugins := loadPluginDirs()
	return &Config{
		Root:       root,
		ConfigDir:  configDir,
		Targets:    targets,
		PluginDirs: plugins,
	}, nil
}

// LoadForDeploy builds a Config without requiring a repository root. The
// deploy command operates on its --repo source (which may not exist yet on the
// target machine), so no local repository is needed.
func LoadForDeploy(configDir string) *Config {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	return &Config{
		ConfigDir:  configDir,
		Targets:    loadTargets(configDir),
		PluginDirs: loadPluginDirs(),
	}
}

// loadTargets reads <configDir>/targets.json and restores built-in defaults
// when missing or invalid (FR-003). Tilde in target paths is expanded to the
// user's home directory.
func loadTargets(configDir string) []common.InstallTarget {
	path := filepath.Join(configDir, targetsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultTargets()
	}
	var targets []common.InstallTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return defaultTargets()
	}
	// Validate each entry; drop malformed ones, expand ~.
	valid := make([]common.InstallTarget, 0, len(targets))
	for _, t := range targets {
		if t.Name == "" || t.Path == "" {
			continue
		}
		if t.Kind != common.KindSkill && t.Kind != common.KindCommand {
			continue
		}
		valid = append(valid, expandTarget(t))
	}
	if len(valid) == 0 {
		return defaultTargets()
	}
	return valid
}

func expandTarget(t common.InstallTarget) common.InstallTarget {
	home, err := os.UserHomeDir()
	if err != nil {
		return t
	}
	if strings.HasPrefix(t.Path, "~/") {
		t.Path = filepath.Join(home, strings.TrimPrefix(t.Path, "~/"))
	}
	return t
}

// loadPluginDirs returns the plugin scan directories: the default dir is
// always scanned first, then any SKM_PLUGINS_DIR entries split on the OS
// path separator (FR-035 / research R8).
func loadPluginDirs() []string {
	dirs := DefaultPluginDirs()
	if env := os.Getenv(EnvPluginsDir); env != "" {
		for _, d := range filepath.SplitList(env) {
			if d != "" {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}
