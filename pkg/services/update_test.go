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
	// Legacy local origins occasionally point to a managed repository entry.
	// That is an internal copy, not an external source P can refresh from.
	writeSvcFile(t, root, "skills/local/internal/SKILL.md", "---\nname: internal\ndescription: internal\n---\nbody\n")
	writeSvcFile(t, root, "skills/local/internal/meta.json", `{"address":"`+filepath.Join(root, "skills", "self-build", "vim")+`","mode_id":"local"}`)

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
	require.Contains(t, res.Skipped, "internal")
	require.Len(t, res.Failed, 1)
	require.Equal(t, "e", res.Failed[0].Name)
	require.NotEmpty(t, res.Failed[0].Reason, "the failure reason must survive, not just the entry name")
	require.NotContains(t, res.Updated, "d", "archived entries are not processed")
	require.Equal(t, 5, res.Total)
}

// TestUpdatable locks the refresh-eligibility rule shared by the CLI
// batch-update and the TUI batch jobs: only active entries with an origin can
// be updated.
func TestUpdatable(t *testing.T) {
	root := t.TempDir()
	svc := newUpdateSvc(t, root)
	unknown := "unknown"
	local := "local"
	selfBuild := "self-build"
	cases := []struct {
		name  string
		entry *common.Entry
		want  bool
	}{
		{"active with origin", &common.Entry{Status: common.StatusActive, Origin: &common.Origin{Address: "/x"}}, true},
		{"adopted unknown origin", &common.Entry{Status: common.StatusActive, ProviderID: &unknown, Origin: &common.Origin{Address: "/x"}}, false},
		{"local origin inside repository", &common.Entry{Status: common.StatusActive, ProviderID: &local, Origin: &common.Origin{Address: filepath.Join(root, "skills", "self-build", "vim")}}, false},
		{"self-built entry with legacy origin", &common.Entry{Status: common.StatusActive, ProviderID: &selfBuild, Origin: &common.Origin{Address: "/x"}}, false},
		{"active without origin", &common.Entry{Status: common.StatusActive}, false},
		{"archived with origin", &common.Entry{Status: common.StatusArchived, Origin: &common.Origin{Address: "/x"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, svc.Updatable(c.entry))
		})
	}
}

func TestUpdateUsesTheRecordedOriginProvider(t *testing.T) {
	root := t.TempDir()
	staged := t.TempDir()
	writeSvcFile(t, root, "skills/recorded/demo/SKILL.md", "---\nname: demo\ndescription: demo\n---\nold\n")
	writeSvcFile(t, root, "skills/recorded/demo/meta.json", `{"address":"shared-source","mode_id":"recorded"}`)
	writeSvcFile(t, staged, "SKILL.md", "---\nname: demo\ndescription: demo\n---\nnew\n")

	svc := newUpdateSvc(t, root)
	// Both providers claim the address, but only the origin's provider must be
	// used. The competing provider is registered first to expose an accidental
	// address-based selection.
	require.NoError(t, svc.Registry.Register(fakeGroupingProvider{id: "competing", staged: t.TempDir()}))
	require.NoError(t, svc.Registry.Register(fakeGroupingProvider{id: "recorded", staged: staged}))

	res, err := svc.Update(context.Background(), "demo", UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Changed)
	data, err := os.ReadFile(filepath.Join(root, "skills/recorded/demo/SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "new")
}

// adapterUpdateFixture builds a command entry with a local origin installed
// into one command-adapter target ("adapter") and one target that has never
// received it ("fresh").
func adapterUpdateFixture(t *testing.T, markerBody string) (*Services, string, string) {
	t.Helper()
	root := t.TempDir()
	writeSvcFile(t, root, "commands/local/demo/command.md", "---\nname: demo\ndescription: demo\n---\n"+markerBody+"\n")
	// Targets live outside the repository root so the scanner does not pick
	// the installed copies up as entries.
	adapterPath := filepath.Join(t.TempDir(), "adapter")
	freshPath := filepath.Join(t.TempDir(), "fresh")
	require.NoError(t, os.MkdirAll(adapterPath, 0o755))
	require.NoError(t, os.MkdirAll(freshPath, 0o755))
	svc, err := New(newCfg(root, []common.InstallTarget{
		skillTarget("adapter", adapterPath),
		skillTarget("fresh", freshPath),
	}), common.NewLogger(false))
	require.NoError(t, err)
	return svc, adapterPath, freshPath
}

// TestUpdateRefreshesCommandAdapterInstalls pins the update-then-refresh
// contract: a command adapter keeps a copy of the entry marker, so replacing
// the entry's content must re-apply the existing install. Targets without a
// prior install are never claimed by an update.
func TestUpdateRefreshesCommandAdapterInstalls(t *testing.T) {
	src := t.TempDir()
	writeSvcFile(t, src, "command.md", "---\nname: demo\ndescription: demo\n---\nnew\n")
	svc, adapterPath, freshPath := adapterUpdateFixture(t, "old")
	writeSvcFile(t, svc.Cfg.Root, "commands/local/demo/meta.json", `{"address":"`+src+`","mode_id":"local"}`)

	ctx := context.Background()
	_, err := svc.Install(ctx, "demo", InstallOptions{Targets: []string{"adapter"}})
	require.NoError(t, err)

	res, err := svc.Update(ctx, "demo", UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Changed)

	data, err := os.ReadFile(filepath.Join(adapterPath, "demo", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "new", "update must refresh the adapter's stale SKILL.md copy")
	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	target, _ := svc.Installer.TargetByName("adapter")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target))
	require.NoDirExists(t, filepath.Join(freshPath, "demo"), "update must not claim a target with no prior install")
}

// TestInstallRefreshesStaleManagedAdapter covers the hand-edited-entry path:
// installing over skm's own but outdated adapter succeeds without --force and
// rewrites the copy, while a foreign occupant still requires --force.
func TestInstallRefreshesStaleManagedAdapter(t *testing.T) {
	svc, adapterPath, _ := adapterUpdateFixture(t, "old")

	ctx := context.Background()
	_, err := svc.Install(ctx, "demo", InstallOptions{})
	require.NoError(t, err)

	writeSvcFile(t, svc.Cfg.Root, "commands/local/demo/command.md", "---\nname: demo\ndescription: demo\n---\nnew\n")
	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	target, _ := svc.Installer.TargetByName("adapter")
	require.Equal(t, common.InstallConflict, svc.Installer.State(entry, target))

	_, err = svc.Install(ctx, "demo", InstallOptions{})
	require.NoError(t, err, "a managed-but-stale adapter is refreshable without --force")
	data, err := os.ReadFile(filepath.Join(adapterPath, "demo", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "new")
	require.Equal(t, common.InstallInstalled, svc.Installer.State(entry, target))

	// A foreign directory in the slot (no adapter marker) is not refreshable.
	require.NoError(t, os.RemoveAll(filepath.Join(adapterPath, "demo")))
	writeSvcFile(t, adapterPath, "demo/SKILL.md", "---\nname: demo\ndescription: foreign\n---\n")
	_, err = svc.Install(ctx, "demo", InstallOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--force")
}
