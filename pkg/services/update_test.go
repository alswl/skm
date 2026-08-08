package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/config"
	"github.com/stretchr/testify/require"
)

func writeSvcFile(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func newUpdateSvc(t *testing.T, root string) *Services {
	t.Helper()
	cfg := &config.Config{
		Root:      root,
		ConfigDir: t.TempDir(),
		Targets:   []common.InstallTarget{},
	}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	return svc
}

func TestBatchUpdateClassifiesResults(t *testing.T) {
	root := t.TempDir()
	// active + origin + changed content -> updated
	srcUpdated := t.TempDir()
	writeSvcFile(t, srcUpdated, "SKILL.md", "---\nname: a\ndescription: a\n---\nnew\n")
	writeSvcFile(t, root, "skills/local/a/SKILL.md", "---\nname: a\ndescription: a\n---\nold\n")
	writeSvcFile(t, root, "skills/local/a/meta.json", `{"address":"`+srcUpdated+`","mode_id":"local"}`)

	// active + origin + identical content -> current
	srcSame := t.TempDir()
	writeSvcFile(t, srcSame, "SKILL.md", "---\nname: b\ndescription: b\n---\nbody\n")
	writeSvcFile(t, root, "skills/local/b/SKILL.md", "---\nname: b\ndescription: b\n---\nbody\n")
	writeSvcFile(t, root, "skills/local/b/meta.json", `{"address":"`+srcSame+`","mode_id":"local"}`)

	// active without origin -> skipped
	writeSvcFile(t, root, "skills/local/c/SKILL.md", "---\nname: c\ndescription: c\n---\nbody\n")

	// archived entry with origin -> not processed (active-only)
	writeSvcFile(t, root, "archived/local/d/SKILL.md", "---\nname: d\ndescription: d\n---\nbody\n")
	writeSvcFile(t, root, "archived/local/d/meta.json", `{"address":"/gone","mode_id":"local"}`)

	// active with origin pointing at an unhandleable address -> failed
	writeSvcFile(t, root, "skills/local/e/SKILL.md", "---\nname: e\ndescription: e\n---\nbody\n")
	writeSvcFile(t, root, "skills/local/e/meta.json", `{"address":"/does/not/exist/anywhere","mode_id":"local"}`)

	svc := newUpdateSvc(t, root)
	res := svc.BatchUpdate(context.Background(), false)

	require.Contains(t, res.Updated, "a")
	require.Contains(t, res.Current, "b")
	require.Contains(t, res.Skipped, "c")
	require.Len(t, res.Failed, 1)
	require.Equal(t, "e", res.Failed[0].Name)
	require.NotEmpty(t, res.Failed[0].Reason, "the failure reason must survive, not just the entry name")
	require.NotContains(t, res.Updated, "d", "archived entries are not processed")
	require.Equal(t, 4, res.Total)
}
