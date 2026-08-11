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

// fakeGroupingProvider is a minimal in-process Provider implementing the
// optional grouper interface, used to verify Services.Import picks up
// Group(address) and threads it through to the on-disk layout without any
// real network fetch (gitHostProvider's own Group derivation is covered
// separately in provider_github_test.go).
type fakeGroupingProvider struct {
	id, group, staged, normalized string
}

func (f fakeGroupingProvider) ID() string    { return f.id }
func (f fakeGroupingProvider) Label() string { return "fake" }
func (f fakeGroupingProvider) Capability() Capability {
	return Capability{ID: f.id, Label: "fake"}
}
func (f fakeGroupingProvider) Normalize(address string) (string, error) {
	if f.normalized != "" {
		return f.normalized, nil
	}
	return address, nil
}
func (f fakeGroupingProvider) CanHandle(address string) bool { return true }
func (f fakeGroupingProvider) Fetch(_ context.Context, _ string) (string, error) {
	return f.staged, nil
}
func (f fakeGroupingProvider) Group(_ string) string { return f.group }

// TestImportPicksUpProviderGroupViaOptionalInterface: Services.Import must
// thread a provider's Group(address) (the grouper optional interface) through
// resolveSource -> ImportStaged, landing the entry under
// <provider>/<group>/<name> — the actual glue that makes gitHostProvider.Group
// visible end-to-end, exercised here with a fake provider so the assertion
// doesn't depend on network access.
func TestImportPicksUpProviderGroupViaOptionalInterface(t *testing.T) {
	root := t.TempDir()
	staged := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staged, "SKILL.md"),
		[]byte("---\nname: widget\ndescription: a widget\n---\nbody\n"), 0o644))

	cfg := &config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	require.NoError(t, svc.Registry.Register(fakeGroupingProvider{id: "fakegroup", group: "acme/widgets", staged: staged, normalized: "normalized/address"}))

	res, err := svc.Import(context.Background(), "acme/widgets", ImportOptions{Provider: "fakegroup"})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "fakegroup", "acme", "widgets", "widget"), res.Path)

	entries := svc.Repo.Scan()
	require.Len(t, entries, 1)
	require.Equal(t, "acme/widgets", entries[0].GroupValue())
	require.NotNil(t, res.Origin)
	require.Equal(t, "normalized/address", res.Origin.Address)
}

func TestImportNormalizesTildeAndPastedWhitespaceForLocalSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "my-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"),
		[]byte("---\nname: local-skill\ndescription: local\n---\nbody\n"), 0o644))

	cfg := &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	res, err := svc.Import(context.Background(), "  ~/my-skill  ", ImportOptions{})
	require.NoError(t, err)
	require.Equal(t, "local", res.Provider)
	require.Equal(t, filepath.Join(cfg.Root, "skills", "local", "local-skill"), res.Path)
	stored, err := dal.ReadMeta(res.Path)
	require.NoError(t, err)
	require.Equal(t, source, stored.Address)
	require.NotNil(t, stored.ProviderID)
	require.Equal(t, "local", *stored.ProviderID)
	require.Equal(t, "skills/local/local-skill", stored.Path)
}

func TestImportWithExplicitLocalProviderPreservesExternalSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "d2")
	require.NoError(t, os.MkdirAll(source, 0o755))
	marker := filepath.Join(source, "SKILL.md")
	require.NoError(t, os.WriteFile(marker,
		[]byte("---\nname: d2\ndescription: local\n---\nbody\n"), 0o644))

	cfg := &config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	res, err := svc.Import(context.Background(), source, ImportOptions{Provider: "local"})
	require.NoError(t, err)
	require.Equal(t, "local", res.Provider)
	require.FileExists(t, filepath.Join(res.Path, "SKILL.md"))
	require.FileExists(t, marker, "explicit local imports must never clean up the caller's source")
	stored, err := dal.ReadMeta(res.Path)
	require.NoError(t, err)
	require.Equal(t, source, stored.Address)
	require.NotNil(t, stored.ProviderID)
	require.Equal(t, "local", *stored.ProviderID)
}

func TestImportRejectsLocalSourceInsideManagedTargetPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "skills", "self-build", "d2")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"),
		[]byte("---\nname: d2\ndescription: local\n---\nbody\n"), 0o644))

	cfg := &config.Config{Root: root, ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	for _, provider := range []string{"", "local"} {
		_, err = svc.Import(context.Background(), source, ImportOptions{Provider: provider})
		require.Error(t, err)
		require.Contains(t, err.Error(), "inside target repository")
	}
	require.FileExists(t, filepath.Join(source, "SKILL.md"))
}

// A local import records the source path as its origin, so update re-fetches
// through the Local provider — which returns that very path. Removing it as
// fetch cleanup would delete the user's source directory.
func TestUpdateOfALocalImportKeepsTheSourceOnDisk(t *testing.T) {
	source := filepath.Join(t.TempDir(), "d2")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"),
		[]byte("---\nname: d2\ndescription: local\n---\nbody\n"), 0o644))

	cfg := &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.Import(context.Background(), source, ImportOptions{})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), "d2", UpdateOptions{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(source, "SKILL.md"), "update must not remove the local source")

	batch := svc.BatchUpdate(context.Background(), false)
	require.Empty(t, batch.Failed)
	require.FileExists(t, filepath.Join(source, "SKILL.md"), "batch-update must not remove the local source")
}

// An explicit --provider local with an address that is a real path but not an
// importable asset fails during probe — after the deferred fetch cleanup was
// registered, which must not point at the caller's own file.
func TestFailedLocalImportKeepsTheSourceOnDisk(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notes.md") // no frontmatter: not an asset
	require.NoError(t, os.WriteFile(file, []byte("just notes\n"), 0o644))

	cfg := &config.Config{Root: t.TempDir(), ConfigDir: t.TempDir(), Targets: []common.InstallTarget{}}
	svc, err := New(cfg, common.NewLogger(false))
	require.NoError(t, err)

	_, err = svc.Import(context.Background(), file, ImportOptions{Provider: "local"})
	require.Error(t, err)
	require.FileExists(t, file, "a failed import must not delete the caller's file")
}
