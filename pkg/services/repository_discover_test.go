package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestDiscoverReportsOnlyRealSkillDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/managed/SKILL.md", frontmatter("managed", "in repo"))

	target := t.TempDir()
	// Real unmanaged skill -> reported.
	writeFile(t, target, "ext-skill/SKILL.md", frontmatter("ext-skill", "external"))
	// Missing SKILL.md -> not reported.
	writeFile(t, target, "no-marker/notes.txt", "x")
	// A managed symlink into the repo -> not reported, never deleted.
	require.NoError(t, os.Symlink(filepath.Join(root, "skills/local/managed"), filepath.Join(target, "managed")))

	targets := []common.InstallTarget{{Name: "t", Path: target, Kind: common.KindSkill}}
	found := NewRepository(root).Discover(targets, "")

	require.Len(t, found, 1, "only the real skill directory is reported")
	require.Equal(t, "ext-skill", found[0].Name)
	// The symlink must be untouched.
	require.True(t, dalIsSymlink(filepath.Join(target, "managed")), "symlinks are never deleted")
}

func dalIsSymlink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}
