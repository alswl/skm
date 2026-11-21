package services

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineCoverage(t *testing.T) {
	doc := config.MachineDocument{
		Directories: map[string]common.MachineDirectory{
			"a": {Tags: []string{"team", "go"}},
			"b": {Tags: []string{"go"}},
		},
		Installs: []common.MachineInstall{
			{Skill: "skills/local/a", Target: "codex", Scope: common.MachineScope{Kind: "tag", Name: "team"}},
			{Skill: "skills/local/a", Target: "codex", Scope: common.MachineScope{Kind: "directory", Name: "b"}},
			{Skill: "skills/local/a", Target: "codex", Scope: common.MachineScope{Kind: "user"}},
		},
	}
	require.True(t, machineDirectoryCovered(doc, "a", "codex", "skills/local/a"))
	require.True(t, machineDirectoryCovered(doc, "b", "codex", "skills/local/a"))
	require.False(t, machineDirectoryCovered(doc, "a", "claude", "skills/local/a"))
	require.False(t, machineDirectoryCovered(doc, "missing", "codex", "skills/local/a"))
	require.True(t, machineUserCovered(doc, "codex", "skills/local/a"))
	require.False(t, machineUserCovered(doc, "claude", "skills/local/a"))
}
