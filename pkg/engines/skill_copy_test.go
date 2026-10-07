package engines

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestCopySkillRollbackRestoresSlot(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "replaced"}[existing], func(t *testing.T) {
			source := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: sample\ndescription: example\n---\nbody"), 0o644))
			target := common.InstallTarget{Path: t.TempDir()}
			dest := filepath.Join(target.Path, "sample")
			if existing {
				require.NoError(t, os.Mkdir(dest, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dest, "original"), []byte("original"), 0o644))
			}
			tx := &dal.FileTransaction{}
			_, err := CopySkill(tx, &common.Entry{Name: "sample", Path: source, Kind: common.KindSkill}, target, existing)
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dest, "SKILL.md"))
			require.NoError(t, tx.Rollback())
			if existing {
				require.FileExists(t, filepath.Join(dest, "original"))
				require.NoFileExists(t, filepath.Join(dest, "SKILL.md"))
			} else {
				require.NoDirExists(t, dest)
			}
		})
	}
}

func TestCopySkillRefusesExistingAndNormalizesFallbackIdentity(t *testing.T) {
	source := filepath.Join(t.TempDir(), "fallback")
	require.NoError(t, os.Mkdir(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: misleading\n---\nbody"), 0o644))
	target := common.InstallTarget{Path: t.TempDir()}
	tx := &dal.FileTransaction{}
	_, err := CopySkill(tx, &common.Entry{Name: "fallback", Path: source, Kind: common.KindSkill}, target, false)
	require.NoError(t, err)
	tx.Commit()
	data, err := os.ReadFile(filepath.Join(target.Path, "fallback/SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name: fallback")
	_, err = CopySkill(&dal.FileTransaction{}, &common.Entry{Name: "fallback", Path: source, Kind: common.KindSkill}, target, false)
	require.Error(t, err)
}
