package services

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestClaimSkillMovesRepositorySkillToSelfBuild(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/broken/SKILL.md", "---\nname: broken\n---\nbody\n")
	cfg, err := config.Load(root, t.TempDir())
	require.NoError(t, err)
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	res, err := svc.ClaimSkill(context.Background(), filepath.Join(root, "skills", "local", "broken"))
	require.NoError(t, err)
	require.Equal(t, "self-build", res.Provider)
	require.Equal(t, filepath.Join(root, "skills", "self-build", "broken"), res.Path)
	require.NoDirExists(t, filepath.Join(root, "skills", "local", "broken"))
}

func TestClaimSkillRejectsExternalSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/ok/SKILL.md", frontmatter("ok", "ok"))
	cfg, err := config.Load(root, t.TempDir())
	require.NoError(t, err)
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	_, err = svc.ClaimSkill(context.Background(), t.TempDir())
	require.ErrorContains(t, err, "outside repository")
}
