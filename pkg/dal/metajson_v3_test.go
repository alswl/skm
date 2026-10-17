package dal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestSkillMetaV3AndLegacySourceCompatibility(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(MetaFilePath(root), []byte(`{"address":"/old/source","mode_id":"local"}`), 0600))
	legacy, err := ReadSkillMeta(root)
	require.NoError(t, err)
	require.Zero(t, legacy.SchemaVersion)
	require.Equal(t, "/old/source", legacy.Address)

	provider := "github"
	commit := strings.Repeat("a", 40)
	meta := common.SkillMeta{Origin: common.Origin{Address: "https://github.com/a/b", ProviderID: &provider, RepositoryAddress: "https://github.com/a/b.git", RequestedRef: "main", Subpath: "skills/demo", Revision: commit, Path: "skills/github/a/b/demo"}, SchemaVersion: 3, RevisionKind: "git", Active: "fork"}
	require.NoError(t, WriteSkillMeta(root, meta))
	got, err := ReadSkillMeta(root)
	require.NoError(t, err)
	require.Equal(t, meta, *got)
	oldReader, err := ReadMeta(root)
	require.NoError(t, err)
	require.Equal(t, commit, oldReader.Revision)

	fork := filepath.Join(t.TempDir(), "fork")
	require.NoError(t, os.Mkdir(fork, 0700))
	require.NoError(t, WriteForkMeta(fork, common.ForkMeta{SchemaVersion: 3, BaseRevision: commit}))
	base, err := ReadForkMeta(fork)
	require.NoError(t, err)
	require.Equal(t, commit, base.BaseRevision)
}

func TestSkillMetaV3RejectsUnprovenVersionAndUnsafePath(t *testing.T) {
	root := t.TempDir()
	for _, meta := range []common.SkillMeta{
		{Origin: common.Origin{Address: "/source", Path: "../outside"}, SchemaVersion: 3, RevisionKind: "snapshot", Active: "upstream"},
		{Origin: common.Origin{Address: "/source", Revision: "made-up"}, SchemaVersion: 3, RevisionKind: "snapshot", Active: "upstream"},
		{Origin: common.Origin{Address: "https://example.com/repo", RepositoryAddress: "https://example.com/repo", Revision: "main"}, SchemaVersion: 3, RevisionKind: "git", Active: "upstream"},
		{Origin: common.Origin{Address: "/source"}, SchemaVersion: 3, RevisionKind: "snapshot", Active: "fork"},
	} {
		if meta.Active == "fork" {
			// Active fork is valid only when its directory exists; that is a
			// repository-level check, so test malformed kind here instead.
			meta.RevisionKind = "unknown"
		}
		require.Error(t, WriteSkillMeta(root, meta))
	}
	require.NoFileExists(t, MetaFilePath(root))
	require.Error(t, WriteForkMeta(root, common.ForkMeta{SchemaVersion: 3, BaseRevision: "main"}))
	require.NoFileExists(t, MetaFilePath(root))
	for _, data := range []string{
		`{}`,
		`{"schema_version":3,"address":"/source","revision_kind":"git","revision":"main","active":"upstream"}`,
		`{"schema_version":3,"address":"/source","revision_kind":"snapshot"}`,
		`{"schema_version":99,"address":"/source"}`,
		`{broken`,
	} {
		require.NoError(t, os.WriteFile(MetaFilePath(root), []byte(data), 0600))
		_, err := ReadSkillMeta(root)
		require.Error(t, err)
	}
}

func TestSkillMetaRejectsSymlinkFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "external.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"address":"/source"}`), 0600))
	require.NoError(t, os.Symlink(target, MetaFilePath(root)))
	_, err := ReadSkillMeta(root)
	require.ErrorContains(t, err, "invalid metadata file")
	err = WriteSkillMeta(root, common.SkillMeta{Origin: common.Origin{Address: "/source"}, SchemaVersion: 3, RevisionKind: "snapshot", Active: "upstream"})
	require.ErrorContains(t, err, "invalid metadata file")
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, `{"address":"/source"}`, string(data))
}
