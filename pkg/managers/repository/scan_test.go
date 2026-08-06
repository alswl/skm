package repository

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestScanMixedRepository(t *testing.T) {
	root := buildFixtureRepo(t)
	entries := New(root).Scan()

	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	// Valid skill
	skillA := byName["skill-a"]
	require.NotNil(t, skillA)
	require.Equal(t, common.KindSkill, skillA.Kind)
	require.Equal(t, common.StatusActive, skillA.Status)
	require.Equal(t, "local", skillA.ModeIDValue())
	require.Nil(t, skillA.Error)

	// Grouped skill
	skillB := byName["skill-b"]
	require.NotNil(t, skillB)
	require.Equal(t, "team", skillB.GroupValue())

	// Remote directory command keeps its origin
	cmdA := byName["cmd-a"]
	require.NotNil(t, cmdA)
	require.Equal(t, common.KindCommand, cmdA.Kind)
	require.Equal(t, "github", cmdA.ModeIDValue())
	require.NotNil(t, cmdA.Origin)
	require.Equal(t, "https://github.com/x/y", cmdA.Origin.Address)

	// Single-file command falls back to the file stem for its name (FR-005)
	single := byName["single"]
	require.NotNil(t, single)
	require.Equal(t, common.KindCommand, single.Kind)
	require.Equal(t, "single", single.Name)

	// Archived entry
	old := byName["old-skill"]
	require.NotNil(t, old)
	require.Equal(t, common.StatusArchived, old.Status)
	require.Equal(t, common.KindSkill, old.Kind)

	// Bad entry surfaces as error without aborting the scan (FR-006)
	bad := byName["bad-marker"]
	require.NotNil(t, bad)
	require.Equal(t, common.StatusError, bad.Status)
	require.NotNil(t, bad.Error)
}

func TestScanModeIDMismatchIsError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/weird/SKILL.md", frontmatter("weird", "origin says local"))
	writeFile(t, root, "skills/github/weird/meta.json", `{"address":"https://x","mode_id":"local"}`)

	entries := New(root).Scan()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, common.StatusError, e.Status, "mode_id mismatch must be an error entry")
	require.Contains(t, *e.Error, "mode_id mismatch")
}

func TestScanMissingMarkerIsError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/empty/SKILL.md", "no frontmatter at all\n")

	entries := New(root).Scan()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, common.StatusError, e.Status)
}

func TestScanEmptyRepository(t *testing.T) {
	root := t.TempDir()
	require.Empty(t, New(root).Scan())
}
