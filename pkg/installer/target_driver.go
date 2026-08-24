package installer

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

// TargetDriver is the installation-side contract shared by built-in and
// external target implementations.
type TargetDriver interface {
	Install(*dal.FileTransaction, *common.Entry, common.InstallTarget, bool) (bool, error)
	Uninstall(*dal.FileTransaction, *common.Entry, common.InstallTarget) (bool, error)
	RemoveForeign(*dal.FileTransaction, *common.Entry, common.InstallTarget) (bool, error)
	State(*common.Entry, common.InstallTarget) (common.InstallState, error)
	Diff(context.Context, *common.Entry, common.InstallTarget) (string, error)
	Inspect(context.Context, common.InstallTarget) ([]DanglingInstall, error)
	RepairDangling(context.Context, DanglingInstall, common.InstallTarget) error
}

type builtinTargetDriver struct{ strategy common.InstallStrategy }

func (d builtinTargetDriver) Install(tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget, force bool) (bool, error) {
	switch d.strategy {
	case common.StrategySkillSymlink:
		return engines.InstallSkill(tx, e, t, force)
	case common.StrategyCommandSymlink:
		return engines.InstallDirectory(tx, e, t, force)
	case common.StrategyCommandMarker:
		return engines.InstallClaudeMarkdown(tx, e, t, force)
	case common.StrategyCommandAdapter:
		return engines.InstallAdapter(tx, e, t, force)
	}
	return false, fmt.Errorf("unsupported target strategy %q", d.strategy)
}

func (d builtinTargetDriver) Uninstall(tx *dal.FileTransaction, e *common.Entry, t common.InstallTarget) (bool, error) {
	switch d.strategy {
	case common.StrategySkillSymlink:
		return engines.UninstallSkill(tx, e, t)
	case common.StrategyCommandSymlink:
		return engines.UninstallDirectory(tx, e, t)
	case common.StrategyCommandMarker:
		return engines.UninstallClaudeMarkdown(tx, e, t)
	case common.StrategyCommandAdapter:
		return engines.UninstallAdapter(tx, e, t)
	}
	return false, fmt.Errorf("unsupported target strategy %q", d.strategy)
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
	out, err := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-ext-diff", "--", e.Path, dest).CombinedOutput()
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
