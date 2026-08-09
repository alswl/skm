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

// fakeGroupingProvider is a minimal in-process Provider implementing the
// optional grouper interface, used to verify Services.Import picks up
// Group(address) and threads it through to the on-disk layout without any
// real network fetch (gitHostProvider's own Group derivation is covered
// separately in provider_github_test.go).
type fakeGroupingProvider struct {
	id, group, staged string
}

func (f fakeGroupingProvider) ID() string    { return f.id }
func (f fakeGroupingProvider) Label() string { return "fake" }
func (f fakeGroupingProvider) Capability() Capability {
	return Capability{ID: f.id, Label: "fake"}
}
func (f fakeGroupingProvider) Normalize(address string) (string, error) { return address, nil }
func (f fakeGroupingProvider) CanHandle(address string) bool            { return true }
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
	require.NoError(t, svc.Registry.Register(fakeGroupingProvider{id: "fakegroup", group: "acme/widgets", staged: staged}))

	res, err := svc.Import(context.Background(), "acme/widgets", ImportOptions{Provider: "fakegroup"})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "skills", "fakegroup", "acme", "widgets", "widget"), res.Path)

	entries := svc.Repo.Scan()
	require.Len(t, entries, 1)
	require.Equal(t, "acme/widgets", entries[0].GroupValue())
}
