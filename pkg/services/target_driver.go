package services

import (
	"context"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/installer"
)

// TargetDriver is re-exported for callers that still construct Services
// dependencies directly; implementation ownership lives in pkg/installer.
type TargetDriver = installer.TargetDriver

type TargetPluginDriver interface {
	installer.TargetDriver
	ID() string
	Label() string
	Capability() TargetPluginCapability
}

// externalTargetDriver adapts the subprocess target plugin to the installer
// domain contract while keeping plugin protocol details in services.
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
func (d externalTargetDriver) State(e *common.Entry, t common.InstallTarget) (common.InstallState, error) {
	return d.TargetPlugin.State(e, t)
}
func (d externalTargetDriver) Diff(ctx context.Context, e *common.Entry, t common.InstallTarget) (string, error) {
	return d.TargetPlugin.Diff(ctx, e, t)
}
func (d externalTargetDriver) Inspect(ctx context.Context, t common.InstallTarget) ([]DanglingInstall, error) {
	return d.TargetPlugin.Inspect(ctx, t)
}
func (d externalTargetDriver) RepairDangling(ctx context.Context, item DanglingInstall, t common.InstallTarget) error {
	return d.TargetPlugin.RepairDangling(ctx, item, t)
}
