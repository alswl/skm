package engines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestScanFinalDirectoriesAsOneSkill(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("skills", "github", "acme", "review")
	upstream := filepath.Join(root, rel)
	fork := filepath.Join(root, dal.SyncForkRel(rel))
	for _, path := range []string{upstream, fork} {
		require.NoError(t, os.MkdirAll(path, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: review\ndescription: Review skill\n---\n"), 0600))
	}
	provider := "github"
	commit := strings.Repeat("a", 40)
	meta := common.SkillMeta{Origin: common.Origin{Address: "https://github.com/acme/skills", RepositoryAddress: "https://github.com/acme/skills.git", Revision: commit, Path: rel, ProviderID: &provider}, SchemaVersion: 3, RevisionKind: "git", Active: "fork"}
	require.NoError(t, dal.WriteSkillMeta(upstream, meta))
	require.NoError(t, dal.WriteForkMeta(fork, common.ForkMeta{SchemaVersion: 3, BaseRevision: commit}))
	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, fork, entries[0].Path)
	require.Equal(t, upstream, entries[0].RepositoryPath)
	require.Equal(t, upstream, entries[0].AlternatePath)
	require.Equal(t, "active", entries[0].SyncView.ForkState)

	meta.Active = "upstream"
	require.NoError(t, dal.WriteSkillMeta(upstream, meta))
	entries = NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, upstream, entries[0].Path)
	require.Equal(t, fork, entries[0].AlternatePath)
	require.Equal(t, "retained", entries[0].SyncView.ForkState)
}

func TestScanFinalSkillRefusesMissingActiveFork(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("skills", "local", "review")
	upstream := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(upstream, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "SKILL.md"), []byte("---\nname: review\ndescription: Review skill\n---\n"), 0600))
	provider := "local"
	require.NoError(t, dal.WriteSkillMeta(upstream, common.SkillMeta{Origin: common.Origin{Address: "/source", ProviderID: &provider, Path: rel}, SchemaVersion: 3, RevisionKind: "snapshot", Active: "fork"}))
	entries := NewRepository(root).Scan()
	require.Len(t, entries, 1)
	require.Equal(t, common.StatusError, entries[0].Status)
	require.NotNil(t, entries[0].Error)
	require.Contains(t, *entries[0].Error, "active fork directory missing")
}
