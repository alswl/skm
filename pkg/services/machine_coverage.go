package services

import (
	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
)

// machineDirectoryCovered checks whether any declaration selects the given
// skill and target for a registered directory.
func machineDirectoryCovered(doc config.InstallationsDocument, directory, target, skill string) bool {
	entry, ok := doc.Directories[directory]
	if !ok {
		return false
	}
	for _, install := range doc.Installs {
		if install.Skill != skill || install.Target != target {
			continue
		}
		switch install.Scope.Kind {
		case "directory":
			if install.Scope.Name == directory {
				return true
			}
		case "tag":
			if containsMachineTag(entry.Tags, install.Scope.Name) {
				return true
			}
		}
	}
	return false
}

// machineUserCovered checks a user-level target independently of directories.
func machineUserCovered(doc config.InstallationsDocument, target, skill string) bool {
	wanted := common.Installation{
		Skill: skill, Target: target, Scope: common.InstallationScope{Kind: "user"},
	}
	for _, install := range doc.Installs {
		if install == wanted {
			return true
		}
	}
	return false
}
