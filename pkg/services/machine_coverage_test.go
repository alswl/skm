package services

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMachineCoverage(t *testing.T) {
	doc := config.InstallationsDocument{
		Directories: map[string]common.Directory{
			"a": {Tags: []string{"team", "go"}},
			"b": {Tags: []string{"go"}},
		},
		Installs: []common.Installation{
			{Skill: "skills/local/a", Target: "codex", Scope: common.InstallationScope{Kind: "tag", Name: "team"}},
			{Skill: "skills/local/a", Target: "codex", Scope: common.InstallationScope{Kind: "directory", Name: "b"}},
			{Skill: "skills/local/a", Target: "codex", Scope: common.InstallationScope{Kind: "user"}},
		},
	}
	require.True(t, machineDirectoryCovered(doc, "a", "codex", "skills/local/a"))
	require.True(t, machineDirectoryCovered(doc, "b", "codex", "skills/local/a"))
	require.False(t, machineDirectoryCovered(doc, "a", "claude", "skills/local/a"))
	require.False(t, machineDirectoryCovered(doc, "missing", "codex", "skills/local/a"))
	require.True(t, machineUserCovered(doc, "codex", "skills/local/a"))
	require.False(t, machineUserCovered(doc, "claude", "skills/local/a"))
}
