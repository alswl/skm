package services

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// PluginInfo is one installed plugin: a file (usually a symlink) in the
// managed plugin directory.
type PluginInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // provider | target
	Path string `json:"path"` // where it lives in the plugin dir
	// Source is the symlink's destination; empty for a regular file copied
	// in by hand.
	Source string `json:"source,omitempty"`
	// Broken marks a link whose destination is gone or not executable — the
	// plugin silently stops loading otherwise.
	Broken bool `json:"broken"`
}

// PluginListResult is the CLI JSON report for `skm plugin list`.
type PluginListResult struct {
	Dir     string       `json:"dir"`
	Plugins []PluginInfo `json:"plugins"`
}

// pluginKindDirs maps the --kind value to its subdirectory.
var pluginKindDirs = map[string]string{"provider": "providers", "target": "targets"}

// PluginDir is the plugin directory skm manages: the first configured base
// (~/.config/skm/plugins by default). Extra dirs from SKM_PLUGINS_DIR are
// scanned at startup but never written to.
func (s *Services) PluginDir() string {
	if len(s.Cfg.PluginDirs) > 0 {
		return s.Cfg.PluginDirs[0]
	}
	return filepath.Join(s.Cfg.ConfigDir, "plugins")
}

// PluginAdd links a local plugin executable into the managed plugin
// directory. kind may be empty when source sits in a providers/ or targets/
// directory, which names the kind already. An existing entry of the same name
// is only replaced with force.
func (s *Services) PluginAdd(source, kind, name string, force bool) (*PluginInfo, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %w", err), common.ExitError)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %w", err), common.ExitError)
	}
	if fi.IsDir() {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %s is a directory; point at the plugin executable itself", abs), common.ExitError)
	}
	if !plugins.IsExecutable(abs) {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %s is not executable; chmod +x it first", abs), common.ExitError)
	}
	if kind == "" {
		if kind, err = inferPluginKind(abs); err != nil {
			return nil, common.WithExitCode(err, common.ExitError)
		}
	}
	sub, ok := pluginKindDirs[kind]
	if !ok {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: unknown kind %q (want provider or target)", kind), common.ExitError)
	}
	if name == "" {
		name = filepath.Base(abs)
	}

	dir := filepath.Join(s.PluginDir(), sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %w", err), common.ExitError)
	}
	dest := filepath.Join(dir, name)
	if _, err := os.Lstat(dest); err == nil {
		if !force {
			return nil, common.WithExitCode(fmt.Errorf("plugin add: %s already exists; pass --force to replace it", dest), common.ExitError)
		}
		if err := os.Remove(dest); err != nil {
			return nil, common.WithExitCode(fmt.Errorf("plugin add: %w", err), common.ExitError)
		}
	}
	if err := os.Symlink(abs, dest); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("plugin add: %w", err), common.ExitError)
	}
	return &PluginInfo{Name: name, Kind: kind, Path: dest, Source: abs, Broken: false}, nil
}

// PluginList reports the plugins installed in the managed plugin directory,
// providers first, each in directory order.
func (s *Services) PluginList() *PluginListResult {
	res := &PluginListResult{Dir: s.PluginDir(), Plugins: []PluginInfo{}}
	for _, kind := range []string{"provider", "target"} {
		dir := filepath.Join(res.Dir, pluginKindDirs[kind])
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // a kind with no directory yet simply has no plugins
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			info := PluginInfo{Name: e.Name(), Kind: kind, Path: path, Broken: !plugins.IsExecutable(path)}
			if target, err := os.Readlink(path); err == nil {
				info.Source = target
			}
			res.Plugins = append(res.Plugins, info)
		}
	}
	return res
}

// PluginRemove deletes a plugin from the managed plugin directory. kind may
// be empty when the name is unambiguous. A regular file (copied in rather
// than linked) is only deleted with force, since its content lives nowhere
// else.
func (s *Services) PluginRemove(name, kind string, force bool) (*PluginInfo, error) {
	var found []PluginInfo
	for _, p := range s.PluginList().Plugins {
		if p.Name == name && (kind == "" || p.Kind == kind) {
			found = append(found, p)
		}
	}
	switch {
	case len(found) == 0:
		return nil, common.WithExitCode(fmt.Errorf("plugin remove: %q not found in %s", name, s.PluginDir()), common.ExitError)
	case len(found) > 1:
		return nil, common.WithExitCode(fmt.Errorf("plugin remove: %q is both a provider and a target; pass --kind", name), common.ExitError)
	}
	p := found[0]
	if p.Source == "" && !force {
		return nil, common.WithExitCode(fmt.Errorf("plugin remove: %s is a regular file, not a link; pass --force to delete it", p.Path), common.ExitError)
	}
	if err := os.Remove(p.Path); err != nil {
		return nil, common.WithExitCode(fmt.Errorf("plugin remove: %w", err), common.ExitError)
	}
	return &p, nil
}

// inferPluginKind reads the kind off the source's own directory name, which
// is how plugin repos already organize them (…/providers/acme).
func inferPluginKind(abs string) (string, error) {
	parent := filepath.Base(filepath.Dir(abs))
	for kind, sub := range pluginKindDirs {
		if parent == sub {
			return kind, nil
		}
	}
	return "", fmt.Errorf("plugin add: cannot tell whether %s is a provider or a target; pass --kind", abs)
}
