package services

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
)

// TargetDriver is the typed target half of the shared Plugin host. Both the
// three built-in strategies and external target executables implement it.
type TargetDriver interface {
	Plugin
	ID() string
	Label() string
	Capability() TargetPluginCapability
	Install(*dal.FileTransaction, *common.Entry, common.InstallTarget, bool) (bool, error)
	Uninstall(*dal.FileTransaction, *common.Entry, common.InstallTarget) (bool, error)
	RemoveForeign(*dal.FileTransaction, *common.Entry, common.InstallTarget) (bool, error)
	State(*common.Entry, common.InstallTarget) (common.InstallState, error)
	Diff(context.Context, *common.Entry, common.InstallTarget) (string, error)
	Inspect(context.Context, common.InstallTarget) ([]DanglingInstall, error)
	RepairDangling(context.Context, DanglingInstall, common.InstallTarget) error
}

// externalTargetDriver preserves the public TargetPlugin methods used by
// existing plugin authors while adapting them to the common TargetDriver
// signature (which carries the transaction used by built-in strategies).
type externalTargetDriver struct{ *TargetPlugin }

func (d externalTargetDriver) Install(_ *dal.FileTransaction, e *common.Entry, t common.InstallTarget, force bool) (bool, error) {
	return d.TargetPlugin.Install(e, t, force)
}
func (d externalTargetDriver) Uninstall(_ *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	return d.TargetPlugin.Uninstall(e, t)
}
func (d externalTargetDriver) RemoveForeign(_ *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	return d.TargetPlugin.RemoveForeign(e, t)
}
func (d externalTargetDriver) Inspect(ctx context.Context, t common.InstallTarget) ([]DanglingInstall, error) {
	return d.TargetPlugin.Inspect(ctx, t)
}
func (d externalTargetDriver) RepairDangling(ctx context.Context, item DanglingInstall, t common.InstallTarget) error {
	return d.TargetPlugin.RepairDangling(ctx, item, t)
}

// builtinTargetDriver adapts the builtin strategy engines into the exact same
// target plugin contract external plugins use.
type builtinTargetDriver struct {
	strategy common.InstallStrategy
}

func (d builtinTargetDriver) ID() string    { return "builtin:" + string(d.strategy) }
func (d builtinTargetDriver) Label() string { return string(d.strategy) }
func (d builtinTargetDriver) Capability() TargetPluginCapability {
	kinds := []common.EntryKind{common.KindSkill, common.KindCommand}
	if d.strategy == common.StrategySkillSymlink {
		kinds = []common.EntryKind{common.KindSkill}
	}
	if d.strategy == common.StrategyCommandMarker || d.strategy == common.StrategyCommandAdapter {
		kinds = []common.EntryKind{common.KindCommand}
	}
	return TargetPluginCapability{ID: d.ID(), Label: d.Label(), Description: "Built-in target strategy", Kinds: kinds}
}
func (d builtinTargetDriver) Descriptor() PluginDescriptor {
	return PluginDescriptor{Version: pluginProtocolVersion, Kind: PluginKindTarget, ID: d.ID(), Label: d.Label(), Description: d.Capability().Description}
}
func (d builtinTargetDriver) Handle(ctx context.Context, req PluginRequest) (PluginResponse, error) {
	if req.Action == "describe" {
		return PluginResponse{Descriptor: d.Descriptor()}, nil
	}
	if req.Entry == nil || req.Target == nil {
		return PluginResponse{}, fmt.Errorf("target plugin %q: entry and target are required", d.ID())
	}
	switch req.Action {
	case "state":
		state, err := d.State(req.Entry, *req.Target)
		return PluginResponse{State: state}, err
	case "diff":
		diff, err := d.Diff(ctx, req.Entry, *req.Target)
		return PluginResponse{Diff: diff}, err
	case "install":
		changed, err := d.Install(nil, req.Entry, *req.Target, req.Force)
		return PluginResponse{Result: &changed}, err
	case "uninstall":
		changed, err := d.Uninstall(nil, req.Entry, *req.Target)
		return PluginResponse{Result: &changed}, err
	default:
		return PluginResponse{}, unsupportedPluginAction(d.Descriptor(), req.Action)
	}
}
func (d builtinTargetDriver) Install(tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget, force bool) (bool, error) {
	switch d.strategy {
	case common.StrategySkillSymlink:
		return engines.InstallSkill(tx, e, t, force)
	case common.StrategyCommandMarker:
		return engines.InstallClaudeMarkdown(tx, e, t, force)
	case common.StrategyCommandAdapter:
		return engines.InstallAdapter(tx, e, t, force)
	}
	return false, unsupportedPluginAction(d.Descriptor(), "install")
}
func (d builtinTargetDriver) Uninstall(tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	switch d.strategy {
	case common.StrategySkillSymlink:
		return engines.UninstallSkill(tx, e, t)
	case common.StrategyCommandMarker:
		return engines.UninstallClaudeMarkdown(tx, e, t)
	case common.StrategyCommandAdapter:
		return engines.UninstallAdapter(tx, e, t)
	}
	return false, unsupportedPluginAction(d.Descriptor(), "uninstall")
}
func (d builtinTargetDriver) RemoveForeign(tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	return engines.RemoveForeign(d.strategy, tx, e, t)
}
func (d builtinTargetDriver) State(e *common.Entry, t common.InstallTarget) (common.InstallState, error) {
	return engines.State(d.strategy, e, t)
}
func (d builtinTargetDriver) Diff(ctx context.Context, e *common.Entry, t common.InstallTarget) (string, error) {
	dest := filepath.Join(t.Path, e.Name)
	if d.strategy == common.StrategyCommandMarker {
		dest += ".md"
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-ext-diff", "--", e.Path, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return "", err
		}
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		return text, nil
	}
	return "content is identical; replacing it with a managed install still changes ownership", nil
}

func (d builtinTargetDriver) Inspect(_ context.Context, t common.InstallTarget) ([]DanglingInstall, error) {
	return engines.InspectDangling(d.strategy, t)
}

func (d builtinTargetDriver) RepairDangling(_ context.Context, item DanglingInstall, _ common.InstallTarget) error {
	return engines.RepairDangling(item)
}
