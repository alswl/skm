package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestImportStagedLocalDirPlacesUnderLocalLayer(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "review")
	writeFile(t, src, "SKILL.md", frontmatter("review", "a review skill"))
	writeFile(t, src, "prompt.txt", "content")

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, "review", res.Name)
	require.Equal(t, common.KindSkill, res.Kind)
	require.Nil(t, res.Origin)

	dest := filepath.Join(root, "skills", "local", "review")
	require.Equal(t, dest, res.Path)
	require.FileExists(t, filepath.Join(dest, "SKILL.md"))
	require.FileExists(t, filepath.Join(dest, "prompt.txt"), "resource files copied")
	_, err = os.Stat(filepath.Join(dest, "meta.json"))
	require.True(t, os.IsNotExist(err))
	// The source must remain (copied, not moved).
	require.FileExists(t, filepath.Join(src, "SKILL.md"))
}

func TestImportStagedAsRejectsPathTraversalEntryID(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "skill")
	writeFile(t, src, "SKILL.md", frontmatter("skill", "a skill"))

	_, err := NewRepository(root).ImportStagedAs(context.Background(), src, "unknown", "target", "..", true, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid entry id")
	require.NoDirExists(t, filepath.Join(root, "skills", "unknown"))
}

func TestImportLocalSkillMarkerFileImportsEnclosingSkill(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "d2")
	writeFile(t, src, "SKILL.md", frontmatter("d2", "a diagram skill"))
	writeFile(t, src, "reference.md", "skill resource")

	cfg := &config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	res, err := svc.Import(context.Background(), filepath.Join(src, "SKILL.md"), ImportOptions{})
	require.NoError(t, err)
	require.Equal(t, common.KindSkill, res.Type)
	require.Equal(t, filepath.Join(root, "skills", "local", "d2"), res.Path)
	require.FileExists(t, filepath.Join(res.Path, "SKILL.md"))
	require.FileExists(t, filepath.Join(res.Path, "reference.md"), "the skill directory is imported, not only its marker")
	require.FileExists(t, filepath.Join(src, "SKILL.md"), "the external marker remains in place")
}

func TestImportSingleFileMarkdownBecomesDirCommand(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "hello.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nname: hello\ndescription: greets\n---\nhi\n"), 0o644))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, common.KindCommand, res.Kind)
	require.Equal(t, filepath.Join(root, "commands", "local", "hello"), res.Path)
	require.FileExists(t, filepath.Join(res.Path, "command.md"))
}

func TestImportProviderRecordsOrigin(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "fetched")
	writeFile(t, staged, "SKILL.md", frontmatter("remote-skill", "from a remote"))
	mode := "github"
	origin := &common.Origin{Address: "https://github.com/x/y", ProviderID: &mode}

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "", false, origin)
	require.NoError(t, err)
	require.Equal(t, "remote-skill", res.Name)
	require.Equal(t, filepath.Join(root, "skills", "github", "remote-skill"), res.Path)
	require.NotNil(t, res.Origin)
	require.Equal(t, "https://github.com/x/y", res.Origin.Address)
	require.Equal(t, "github", *res.Origin.ProviderID)
	require.Equal(t, "skills/github/remote-skill", res.Origin.Path, "meta.json tracks the installed path relative to the repo root")
	require.FileExists(t, filepath.Join(res.Path, "meta.json"))

	// The on-disk meta.json carries url / provider / path for management.
	stored, err := dal.ReadMeta(res.Path)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/x/y", stored.Address)
	require.Equal(t, "github", *stored.ProviderID)
	require.Equal(t, "skills/github/remote-skill", stored.Path)
}

// TestImportWithGroupNestsUnderOwnerRepoAndScanReportsIt: a non-empty group
// (as gitHostProvider.Group derives for a GitHub/GitLab address) places the
// import under <provider>/<group>/<name> instead of the flat
// <provider>/<name> layout, so the entry's Group is recovered on the next
// scan the same way any other nested provider directory already is
// (repository_scan.go scanGroup) — "GitHub 要显示 group/repo/name".
func TestImportWithGroupNestsUnderOwnerRepoAndScanReportsIt(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(t.TempDir(), "fetched")
	writeFile(t, staged, "SKILL.md", frontmatter("remote-skill", "from a remote"))
	mode := "github"
	origin := &common.Origin{Address: "https://github.com/octocat/hello-world", ProviderID: &mode}

	res, err := NewRepository(root).ImportStaged(context.Background(), staged, "github", "octocat/hello-world", false, origin)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "github", "octocat", "hello-world", "remote-skill"), res.Path)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, "remote-skill", entries[0].Name)
	require.Equal(t, "octocat/hello-world", entries[0].GroupValue(), "the scan recovers the owner/repo group from the nested directory layout")
	require.Equal(t, "github", entries[0].ProviderIDValue())
}

func TestImportCollisionRejectedWithoutForce(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/dup/SKILL.md", frontmatter("dup", "first"))
	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "second"))

	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.Error(t, err, "same destination collision must be rejected without force")
	// Original intact.
	require.FileExists(t, filepath.Join(root, "skills/local/dup/SKILL.md"))
}

func TestImportForceOverwritePreservesExternalSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/dup/SKILL.md", frontmatter("dup", "original"))
	orig := filepath.Join(root, "skills/local/dup/SKILL.md")
	before, _ := os.ReadFile(orig)

	externalRoot := t.TempDir()
	src := filepath.Join(externalRoot, "dup")
	sourceMarker := filepath.Join(src, "SKILL.md")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "overwritten"))
	sourceBefore, err := os.ReadFile(sourceMarker)
	require.NoError(t, err)

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", true, nil)
	require.NoError(t, err)
	require.Equal(t, "dup", res.Name)
	require.Equal(t, filepath.Join(root, "skills/local/dup"), res.Path)
	after, _ := os.ReadFile(orig)
	require.NotEqual(t, string(before), string(after), "force overwrites the marker")
	sourceAfter, err := os.ReadFile(sourceMarker)
	require.NoError(t, err)
	require.Equal(t, sourceBefore, sourceAfter, "force-importing into another repository must not modify the source")
}

// TestImportAllowsSameNameAtDifferentPaths preserves path-based identity.
func TestImportAllowsSameNameAtDifferentPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/github/dup/SKILL.md", frontmatter("dup", "flat, pre-group import"))

	src := filepath.Join(t.TempDir(), "dup")
	writeFile(t, src, "SKILL.md", frontmatter("dup", "regrouped"))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "github", "octocat/hello-world", true, nil)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills/github/octocat/hello-world/dup"), res.Path)

	entries := NewRepository(root).Scan()
	require.Len(t, entries, 2)
	require.FileExists(t, filepath.Join(root, "skills/github/dup/SKILL.md"))
	require.FileExists(t, res.Path+"/SKILL.md")
}

func TestImportForceRejectsAnAlreadyManagedSourceWithoutMovingIt(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "skills", "self-build", "d2")
	writeFile(t, root, "skills/self-build/d2/SKILL.md", frontmatter("d2", "self-built diagram skill"))
	before, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	require.NoError(t, err)

	_, err = NewRepository(root).ImportStaged(context.Background(), src, "local", "", true, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already managed")

	after, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, before, after, "the managed source must not be replaced or moved")
	require.NoDirExists(t, filepath.Join(root, "skills", "local", "d2"))
}

func TestImportRejectsUnidentifiableSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "plain-dir")
	writeFile(t, src, "notes.txt", "no marker here")
	_, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot identify")
}

func TestImportRepairsMalformedSkillMetadata(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "skills", "legacy-review")
	writeFile(t, root, "skills/legacy-review/SKILL.md", "---\ndescription: legacy review instructions\n---\nReview the change.\n")

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, "legacy-review", res.Name)
	require.FileExists(t, filepath.Join(root, "skills", "local", "legacy-review", "SKILL.md"))
	require.NoDirExists(t, src, "claimed source is moved instead of duplicated")

	data, err := os.ReadFile(filepath.Join(res.Path, "SKILL.md"))
	require.NoError(t, err)
	fm, body, err := dal.ParseFrontmatter(data)
	require.NoError(t, err)
	require.Equal(t, "legacy-review", fm.Name)
	require.Equal(t, "legacy review instructions", fm.Description)
	require.Contains(t, string(body), "Review the change.")
}

func TestImportRepairsUnparseableSkillMetadataWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "loose-skill")
	original := "---\nname: [not valid\n---\nKeep these instructions.\n"
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "SKILL.md"), []byte(original), 0o644))

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(res.Path, "SKILL.md"))
	require.NoError(t, err)
	fm, body, err := dal.ParseFrontmatter(data)
	require.NoError(t, err)
	require.Equal(t, "loose-skill", fm.Name)
	require.Equal(t, "Imported and normalized skill", fm.Description)
	require.Contains(t, string(body), "Keep these instructions.")
	source, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, original, string(source))
}

func TestImportRepairsInvalidManagedSkillInPlace(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "skills", "local", "broken")
	writeFile(t, root, "skills/local/broken/SKILL.md", "---\nname: broken\n---\nbody\n")

	res, err := NewRepository(root).ImportStaged(context.Background(), src, "local", "", false, nil)
	require.NoError(t, err)
	require.Equal(t, src, res.Path)
	data, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
	require.NoError(t, err)
	fm, _, err := dal.ParseFrontmatter(data)
	require.NoError(t, err)
	require.Equal(t, "broken", fm.Name)
	require.Equal(t, "Imported and normalized skill", fm.Description)
}
