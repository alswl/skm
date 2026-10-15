package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func syncFixture(t *testing.T) (*Services, string, string) {
	t.Helper()
	root := t.TempDir()
	source := t.TempDir()
	target := t.TempDir()
	writeFile(t, source, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, source, "guide.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	svc, err := New(newCfg(root, []common.InstallTarget{{Name: "test", Path: target, Accepts: []common.EntryKind{common.KindSkill}, Strategies: map[common.EntryKind]common.InstallStrategy{common.KindSkill: common.StrategySkillSymlink}}}), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.Import(context.Background(), source, ImportOptions{})
	require.NoError(t, err)
	return svc, source, target
}
func readSyncFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
func currentSyncPath(t *testing.T, s *Services) string {
	t.Helper()
	e, err := s.ResolveEntry("review")
	require.NoError(t, err)
	require.NotNil(t, e)
	return filepath.Join(e.Path, "guide.txt")
}

func rootForSyncTest(path string) string {
	for p := filepath.Dir(path); p != filepath.Dir(p); p = filepath.Dir(p) {
		if filepath.Base(p) == "skills" {
			return filepath.Dir(p)
		}
	}
	return ""
}
func relativeForSyncTest(path string) string {
	rel, _ := filepath.Rel(rootForSyncTest(path), path)
	return rel
}
