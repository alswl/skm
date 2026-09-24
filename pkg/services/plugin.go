package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// Target is the config.yaml target `plugin add` registered for a target
	// plugin that declares a target_path in its capability; nil when nothing
	// was registered.
	Target *common.InstallTarget `json:"target,omitempty"`
	// Hint explains what is still missing when a target plugin was linked but
	// no target could be registered from it — linking alone installs nothing.
	Hint string `json:"hint,omitempty"`
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
	info := &PluginInfo{Name: name, Kind: kind, Path: dest, Source: abs, Broken: false}
	if kind == "target" {
		s.registerPluginTarget(info, dest, force)
	}
	return info, nil
}

// registerPluginTarget turns a freshly linked target plugin into a usable
// target. A target plugin is an install strategy, not a target: without a
// config.yaml entry referencing it as plugin:<id>, linking it changes nothing
// visible (`target list` stays as it was) — the trap a fresh machine falls
// into. When the plugin declares a target_path in its capability, the entry
// is written here; otherwise info.Hint names the `target add` to run.
func (s *Services) registerPluginTarget(info *PluginInfo, path string, force bool) {
	plugin, err := NewTargetPlugin(path)
	if err != nil {
		info.Hint = fmt.Sprintf("linked, but the plugin did not load: %v", err)
		return
	}
	cap := plugin.Capability()
	kinds := cap.Kinds
	if cap.TargetPath == "" || len(kinds) == 0 {
		info.Hint = fmt.Sprintf("a target plugin is an install strategy, not a target — register one that uses it:\n"+
			"  skm target add --name %s --path <install dir> --accepts %s%s",
			cap.ID, kindsList(kinds), strategyFlags(kinds, cap.ID))
		return
	}

	target := common.InstallTarget{
		Name: cap.ID, Platform: cap.ID, Path: cap.TargetPath, Accepts: kinds,
		Strategies: make(map[common.EntryKind]common.InstallStrategy, len(kinds)),
	}
	for _, k := range kinds {
		target.Strategies[k] = common.PluginStrategy(cap.ID)
	}

	registered, err := s.registerTarget(target, force)
	switch {
	case err != nil:
		info.Hint = fmt.Sprintf("linked, but registering target %q failed: %v", cap.ID, err)
	case registered == nil:
		info.Hint = fmt.Sprintf("target %q already exists and was left as it is; pass --force to update it to what the plugin declares", cap.ID)
	default:
		info.Target = registered
	}
}

// registerTarget adds target, or updates the existing entry of that name when
// force is set — re-linking a plugin on a machine that already has its target
// must not fail, and --force is the user asking for the stored entry to match
// what the plugin now declares. An existing entry left untouched returns
// (nil, nil): nothing was written, so nothing may be reported as registered.
func (s *Services) registerTarget(target common.InstallTarget, force bool) (*common.InstallTarget, error) {
	exists := false
	for _, t := range s.Cfg.Targets {
		if t.Name == target.Name {
			exists = true
			break
		}
	}
	var (
		stored common.InstallTarget
		err    error
	)
	switch {
	case !exists:
		stored, err = s.TargetAdd(target)
	case !force:
		return nil, nil
	default:
		stored, err = s.TargetUpdate(target.Name, func(t *common.InstallTarget) {
			t.Path, t.Accepts, t.Strategies = target.Path, target.Accepts, target.Strategies
		})
	}
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

// hintKinds is the kind list the hint's example command uses: what the plugin
// declared, or skill when it declared nothing.
func hintKinds(kinds []common.EntryKind) []common.EntryKind {
	if len(kinds) == 0 {
		return []common.EntryKind{common.KindSkill}
	}
	return kinds
}

// kindsList renders kinds as the comma-separated --accepts value.
func kindsList(kinds []common.EntryKind) string {
	parts := make([]string, 0, len(kinds))
	for _, k := range hintKinds(kinds) {
		parts = append(parts, string(k))
	}
	return strings.Join(parts, ",")
}

// strategyFlags renders one --strategy flag per accepted kind, since every
// accepted kind needs its own strategy for the target to validate.
func strategyFlags(kinds []common.EntryKind, id string) string {
	var b strings.Builder
	for _, k := range hintKinds(kinds) {
		fmt.Fprintf(&b, " --strategy %s=plugin:%s", k, id)
	}
	return b.String()
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
