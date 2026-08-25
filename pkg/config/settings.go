package config

import (
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"gopkg.in/yaml.v3"
)

// SettingsFileName is the optional settings file read from the config
// directory. It is optional: with no file, every setting keeps the default it
// had before the file existed.
const SettingsFileName = "config.yaml"

// EnvRoot is the environment form of the settings file's `root`, equivalent to
// --root.
const EnvRoot = "SKM_ROOT"

// settings is the parsed settings file. Precedence over these values is
// flag > environment variable > file > default (12-factor).
//
// --config is deliberately absent: the settings file lives *in* the config
// directory, so it cannot also choose it. That one stays flag-only, with
// $XDG_CONFIG_HOME deciding the default (see DefaultConfigDir).
type settings struct {
	// Root is the repository root, equivalent to --root. Empty means
	// "discover upward from cwd".
	Root string `yaml:"root"`
	// PluginDirs are scanned *in addition to* the default plugin directory,
	// which is always scanned first. The environment form is
	// SKM_PLUGINS_DIR, an OS-path-list string.
	PluginDirs []string `yaml:"plugin_dirs"`
}

// loadSettings reads <configDir>/config.yaml. A missing file is the normal
// case and yields zero values; a malformed one is an error rather than a
// silent fallback, because a mistyped `root:` would otherwise send every
// command at a different repository without saying so.
func loadSettings(configDir string) (settings, error) {
	var s settings
	path := filepath.Join(configDir, SettingsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, common.Errf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, common.Errf("%s: %w", path, err)
	}
	return s, nil
}

// rootFrom resolves the repository root argument: the --root flag wins, then
// SKM_ROOT, then the settings file, then "" so DiscoverRoot searches upward.
func rootFrom(s settings, rootFlag string) string {
	if rootFlag != "" {
		return rootFlag
	}
	if env := os.Getenv(EnvRoot); env != "" {
		return env
	}
	return s.Root
}

// pluginDirsFrom returns the plugin scan directories: the default dir is
// always scanned first, then the extra dirs. Setting SKM_PLUGINS_DIR (an
// OS-path-list string) replaces the settings file's list, keeping
// flag > env > file. A leading "~/" is expanded in both forms, as it is for
// target paths.
func pluginDirsFrom(s settings) []string {
	dirs := DefaultPluginDirs()
	if env := os.Getenv(EnvPluginsDir); env != "" {
		for _, d := range filepath.SplitList(env) {
			if d != "" {
				dirs = append(dirs, expandHome(d))
			}
		}
		return dirs
	}
	for _, d := range s.PluginDirs {
		if d != "" {
			dirs = append(dirs, expandHome(d))
		}
	}
	return dirs
}
