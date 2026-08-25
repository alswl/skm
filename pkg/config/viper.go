package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// SettingsFileName is the optional settings file read from the config
// directory, in any format viper understands (config.yaml, config.toml,
// config.json). It is optional: with no file, every setting keeps the
// default it had before the file existed.
const SettingsFileName = "config"

// Settings keys. Precedence is flag > environment variable > settings file >
// default (12-factor; docs/go-cli-guides.md "Configuration precedence").
//
// --config is deliberately absent: the settings file lives *in* the config
// directory, so it cannot also choose it. That one stays flag-only, with
// $XDG_CONFIG_HOME deciding the default (see DefaultConfigDir).
const (
	// KeyRoot is the repository root, equivalent to --root. Empty means
	// "discover upward from cwd".
	KeyRoot = "root"
	// KeyPluginDirs holds plugin directories scanned *in addition to* the
	// default one, which is always scanned first. The environment form is
	// SKM_PLUGINS_DIR, an OS-path-list string.
	KeyPluginDirs = "plugin_dirs"

	// EnvRoot is the environment form of KeyRoot.
	EnvRoot = "SKM_ROOT"
)

// newSettings builds the viper instance for configDir. A missing or unreadable
// settings file is not an error — it is the normal case.
func newSettings(configDir string) *viper.Viper {
	v := viper.New()
	v.SetConfigName(SettingsFileName)
	v.AddConfigPath(configDir)
	v.SetDefault(KeyRoot, "")
	v.SetDefault(KeyPluginDirs, []string{})
	_ = v.BindEnv(KeyRoot, EnvRoot)
	_ = v.ReadInConfig()
	return v
}

// rootFrom resolves the repository root argument: the --root flag wins, then
// SKM_ROOT or the settings file, then "" so DiscoverRoot searches upward.
func rootFrom(v *viper.Viper, rootFlag string) string {
	if rootFlag != "" {
		return rootFlag
	}
	return v.GetString(KeyRoot)
}

// pluginDirsFrom returns the plugin scan directories: the default dir is
// always scanned first, then the extra dirs. SKM_PLUGINS_DIR is read directly
// rather than through viper's slice binding, because it is an OS-path-list
// string (":"-separated on Unix) and viper would split it on whitespace.
// Setting it replaces the settings file's list, keeping flag > env > file.
// A leading "~/" is expanded in both forms, as it is for target paths.
func pluginDirsFrom(v *viper.Viper) []string {
	dirs := DefaultPluginDirs()
	if env := os.Getenv(EnvPluginsDir); env != "" {
		for _, d := range filepath.SplitList(env) {
			if d != "" {
				dirs = append(dirs, expandHome(d))
			}
		}
		return dirs
	}
	for _, d := range v.GetStringSlice(KeyPluginDirs) {
		if d != "" {
			dirs = append(dirs, expandHome(d))
		}
	}
	return dirs
}
