package commands

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"

	"github.com/stretchr/testify/require"
)

func TestMachinePlanSummaryDeduplicatesOverlappingLocations(t *testing.T) {
	items := []common.MachinePlanItem{
		{Directory: "a", Target: "codex", Skill: "skills/local/review", Path: "/a/review", Status: "install"},
		{Directory: "a", Target: "codex", Skill: "skills/local/review", Path: "/a/review", Status: "install"},
		{Directory: "b", Target: "codex", Skill: "skills/local/review", Path: "/b/review", Status: "conflict", Error: "occupied"},
	}
	plan, err := makeMachinePlan("restore", "digest", map[string]bool{}, items)
	require.NoError(t, err)
	require.Equal(t, 2, plan.Summary["total"])
	require.Equal(t, 1, plan.Summary["conflict"])
	require.Len(t, plan.Diagnostics, 1)
}

func TestMachinePlanMultipleSkillsTargetsAndTags(t *testing.T) {
	root, cfg := cmdFixture(t)
	writeTestFile(t, root, "skills/local/skill-b/SKILL.md", "---\nname: skill-b\ndescription: B\n---\n")
	for _, id := range []string{"a", "b"} {
		project := filepath.Join(t.TempDir(), id)
		output, err := exec.Command("git", "init", "-q", project).CombinedOutput()
		require.NoError(t, err, string(output))
		_, err = runCmd(t, "machine", "directory", "add", id+"="+project, "--tag", "team", "--root", root, "--config", cfg)
		require.NoError(t, err)
	}
	_, err := runCmd(t, "machine", "directory", "tag", "--directory", "a", "--add", "extra", "--root", root, "--config", cfg)
	require.NoError(t, err)
	output, err := runCmd(t, "machine", "plan", "--add", "skill-a", "--add", "skill-b", "--target", "codex", "--target", "claude", "--tag", "team", "--tag", "extra", "--json", "--root", root, "--config", cfg)
	require.NoError(t, err)
	var plan machinePlanDocument
	require.NoError(t, json.Unmarshal([]byte(output), &plan))
	require.Len(t, plan.Items, 8)
	require.Equal(t, 8, plan.Summary["total"])
}
