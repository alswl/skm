package services

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/installer"
)

type Installer = installer.Installer
type DanglingInstall = installer.DanglingInstall

func NewInstaller(targets []common.InstallTarget, targetPlugins map[string]TargetDriver) *Installer {
	return installer.NewInstaller(targets, targetPlugins)
}
