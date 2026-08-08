package services

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestScanMixedRepository(t *testing.T) {
	root := buildFixtureRepo(t)
	entries := NewRepository(root).Scan()

	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	// Valid skill
	skillA := byName["skill-a"]
	require.NotNil(t, skillA)
	require.Equal(t, common.KindSkill, skillA.Kind)
	require.Equal(t, common.StatusActive, skillA.Status)
	require.Equal(t, "local", skillA.ProviderIDValue())
	require.Nil(t, skillA.Error)

	// Grouped skill
	skillB := byName["skill-b"]
	require.NotNil(t, skillB)
	require.Equal(t, "team", skillB.GroupValue())

	// Remote directory command keeps its origin
	cmdA := byName["cmd-a"]
	require.NotNil(t, cmdA)
	require.Equal(t, common.KindCommand, cmdA.Kind)
	require.Equal(t, "github", cmdA.ProviderIDValue())
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

// TestScanFindsArchivedSingleFileCommand: scanTop/scanProvider pass an empty
// EntryKind for the "archived" tree (it mixes skills and commands), and the
// flat-file branch used to require kind == common.KindCommand exactly — so an
// archived single-file command (archived/<provider>/<name>.md) was silently
// dropped from Scan() entirely, not even reported as an error. The directory
// branch already tolerated the empty kind via hasMarker's permissive default;
// this locks in that the flat-file branch now does too.
func TestScanFindsArchivedSingleFileCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "archived/local/flatcmd.md", frontmatter("flatcmd", "archived single-file command"))

	entries := NewRepository(root).Scan()
	var found *common.Entry
	for _, e := range entries {
		if e.Name == "flatcmd" {
			found = e
		}
	}
	require.NotNil(t, found, "an archived single-file command must still be found by Scan()")
	require.Equal(t, common.KindCommand, found.Kind)
	require.Equal(t, common.StatusArchived, found.Status)
}

func TestScanProviderIDMismatchIsError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/weird/SKILL.md", frontmatter("weird", "origin says local"))
	writeFile(t, root, "skills/github/weird/meta.json", `{"address":"https://x","mode_id":"local"}`)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, common.StatusError, e.Status, "mode_id mismatch must be an error entry")
	require.Contains(t, *e.Error, "mode_id mismatch")
}

func TestScanMissingMarkerIsError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/empty/SKILL.md", "no frontmatter at all\n")

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, common.StatusError, e.Status)
}

func TestScanEmptyRepository(t *testing.T) {
	root := t.TempDir()
	require.Empty(t, NewRepository(root).Scan())
}

// TestScanFlagsMarkerOutsideManagedTrees: a SKILL.md/command.md placed
// outside skills/commands/archived must still be scanned and reported, not
// silently invisible.
func TestScanFlagsMarkerOutsideManagedTrees(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/skill-a/SKILL.md", frontmatter("skill-a", "A skill"))
	writeFile(t, root, "stray-skill/SKILL.md", frontmatter("stray", "misplaced skill"))
	writeFile(t, root, "nested/deeper/stray-cmd/command.md", frontmatter("stray-cmd", "misplaced command"))

	entries := NewRepository(root).Scan()
	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	require.Equal(t, common.StatusActive, byName["skill-a"].Status, "the properly placed skill is unaffected")

	stray := byName["stray"]
	require.NotNil(t, stray, "the misplaced skill is scanned, not ignored")
	require.Equal(t, common.StatusNonStandard, stray.Status)
	require.Equal(t, common.KindSkill, stray.Kind)
	require.Equal(t, "misplaced skill", stray.Description, "frontmatter is still parsed for a non-standard entry")
	require.NotNil(t, stray.Error)
	require.Contains(t, *stray.Error, "non-standard location")

	strayCmd := byName["stray-cmd"]
	require.NotNil(t, strayCmd, "a marker nested several levels deep outside the managed trees is still found")
	require.Equal(t, common.StatusNonStandard, strayCmd.Status)
	require.Equal(t, common.KindCommand, strayCmd.Kind)
}

// TestScanNonStandardFallsBackToDirNameWhenFrontmatterBroken: location is the
// primary problem being reported, so a non-standard entry with unparsable
// frontmatter still surfaces (using the directory name), rather than being
// dropped or misreported as a plain error.
func TestScanNonStandardFallsBackToDirNameWhenFrontmatterBroken(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "loose-dir/SKILL.md", "no frontmatter at all\n")

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, common.StatusNonStandard, e.Status)
	require.Equal(t, "loose-dir", e.Name)
}

// TestScanNonStandardIgnoresHiddenAndLooseMarkdown: dot-directories (e.g.
// .git) are skipped, and a loose *.md file with no directory marker is never
// mistaken for a stray command (avoids false positives from README.md etc.).
func TestScanNonStandardIgnoresHiddenAndLooseMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".git/SKILL.md", frontmatter("should-not-appear", "inside a dot dir"))
	writeFile(t, root, "README.md", "# Not a command\n")

	require.Empty(t, NewRepository(root).Scan())
}

// TestScanFlagsMissingProviderLevel: skills/<name>/SKILL.md (missing the
// required provider directory: skills/<provider>/<name>/SKILL.md) is a real
// pattern people fall into — the top-level directory is silently treated as
// "the provider" and its marker is never found one level deeper. It must be
// scanned and flagged, not invisible.
func TestScanFlagsMissingProviderLevel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/proper/local/proper-skill/SKILL.md", frontmatter("proper-skill", "correctly nested"))
	writeFile(t, root, "skills/flat-skill/SKILL.md", frontmatter("flat-skill", "missing provider level"))
	writeFile(t, root, "archived/flat-archived/SKILL.md", frontmatter("flat-archived", "missing provider level, archived"))

	entries := NewRepository(root).Scan()
	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	require.Equal(t, common.StatusActive, byName["proper-skill"].Status, "correctly nested skill is unaffected")

	flat := byName["flat-skill"]
	require.NotNil(t, flat, "a skill missing its provider directory is scanned, not silently dropped")
	require.Equal(t, common.StatusNonStandard, flat.Status)
	require.Equal(t, common.KindSkill, flat.Kind)
	require.Equal(t, "missing provider level", flat.Description)
	require.Contains(t, *flat.Error, "skills/<provider>")

	flatArchived := byName["flat-archived"]
	require.NotNil(t, flatArchived, "the same gap under archived/ is also caught")
	require.Equal(t, common.StatusNonStandard, flatArchived.Status)
	require.Equal(t, common.KindSkill, flatArchived.Kind, "kind is inferred from whichever marker is present")
}

// TestScanFlagsLooseCommandFileMissingProviderLevel: commands/<name>.md
// (missing the required provider directory: commands/<provider>/<name>.md)
// — a loose single-file command dropped directly under commands/ was
// previously skipped entirely (scanTop only recursed into directories).
func TestScanFlagsLooseCommandFileMissingProviderLevel(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "commands/local/proper-cmd/command.md", frontmatter("proper-cmd", "correctly nested"))
	writeFile(t, root, "commands/loose.md", frontmatter("loose", "missing provider level"))

	entries := NewRepository(root).Scan()
	byName := map[string]*common.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	require.Equal(t, common.StatusActive, byName["proper-cmd"].Status)

	loose := byName["loose"]
	require.NotNil(t, loose, "a loose command file with no provider directory is scanned, not silently dropped")
	require.Equal(t, common.StatusNonStandard, loose.Status)
	require.Equal(t, common.KindCommand, loose.Kind)
	require.Contains(t, *loose.Error, "commands/<provider>/<name>.md")
}
