package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
	targetdefs "github.com/alswl/skm/skm/pkg/targets"
	"github.com/stretchr/testify/require"
)

func newDeploySvc(t *testing.T, targets []common.InstallTarget) *Services {
	t.Helper()
	svc, err := New(newCfg(t.TempDir(), targets), common.NewLogger(false))
	require.NoError(t, err)
	return svc
}

func findEntryIn(t *testing.T, root, name string) *common.Entry {
	t.Helper()
	for _, e := range engines.NewRepository(root).Scan() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entry %q not found", name)
	return nil
}

func deployFixture(t *testing.T) (*Services, DeployOptions, string, []common.InstallTarget) {
	t.Helper()
	base := t.TempDir()
	src := filepath.Join(base, "source with spaces")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeSvcFile(t, src, filepath.Join("nested", name, "SKILL.md"), "---\nname: "+name+"\ndescription: sample\n---\ncontent\n")
		writeSvcFile(t, src, filepath.Join("nested", name, "resource.txt"), "resource "+name)
	}
	targets := []common.InstallTarget{skillTarget("one", filepath.Join(base, "target-one")), skillTarget("two", filepath.Join(base, "target-two"))}
	svc := newDeploySvc(t, targets)
	return svc, DeployOptions{Repo: src, Into: filepath.Join(base, "library"), Targets: []string{"one", "two"}}, src, targets
}

func TestDeployLibraryAndLocalUpdate(t *testing.T) {
	svc, opts, src, targets := deployFixture(t)
	// A local .git directory must not invoke git pull, even without a remote.
	writeSvcFile(t, src, ".git/config", "invalid git config; must not be read")
	writeSvcFile(t, src, "nested/alpha/meta.json", `{"address":"/untrusted","mode_id":"wrong"}`)
	r, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, r.Success)
	require.Equal(t, "direct", r.Clone)
	require.Len(t, r.Operations, 9)
	require.Len(t, r.Results, 6)
	for _, name := range r.Skills {
		for _, target := range targets {
			resolved, err := filepath.EvalSymlinks(filepath.Join(target.Path, name))
			require.NoError(t, err)
			expected, err := filepath.EvalSymlinks(filepath.Join(opts.Into, "skills/local", name))
			require.NoError(t, err)
			require.Equal(t, expected, resolved)
		}
	}
	e := findEntryIn(t, opts.Into, "alpha")
	expectedSource, err := filepath.EvalSymlinks(filepath.Join(src, "nested/alpha"))
	require.NoError(t, err)
	require.Equal(t, expectedSource, e.Origin.Address)
	require.Empty(t, e.Origin.Subpath)
	copySvc := *svc
	cfg := *svc.Cfg
	cfg.Root = opts.Into
	copySvc.Cfg = &cfg
	copySvc.Repo = engines.NewRepository(opts.Into)
	writeSvcFile(t, src, "nested/alpha/resource.txt", "changed")
	u, err := copySvc.Update(context.Background(), "alpha", UpdateOptions{})
	require.NoError(t, err)
	require.True(t, u.Changed)
	require.NoError(t, os.RemoveAll(src))
	for _, target := range targets {
		data, err := os.ReadFile(filepath.Join(target.Path, "alpha/resource.txt"))
		require.NoError(t, err)
		require.Equal(t, "changed", string(data))
	}
	_, err = svc.Deploy(context.Background(), opts)
	require.ErrorContains(t, err, "already exists")
	opts.Force = true
	_, err = svc.Deploy(context.Background(), opts)
	require.ErrorContains(t, err, "already exists")
}

func TestDeployDirectCopiesAndExplicitRefresh(t *testing.T) {
	svc, opts, src, targets := deployFixture(t)
	library := opts.Into
	opts.Into = ""
	opts.NoRepo = true
	writeSvcFile(t, src, "nested/alpha/meta.json", `{"address":"bad"}`)
	r, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, "direct", r.Mode)
	require.Nil(t, r.Destination)
	require.NoDirExists(t, library)
	for _, target := range targets {
		fi, err := os.Lstat(filepath.Join(target.Path, "alpha"))
		require.NoError(t, err)
		require.True(t, fi.IsDir())
		require.NoFileExists(t, filepath.Join(target.Path, "alpha/meta.json"))
	}
	_, err = svc.Deploy(context.Background(), opts)
	require.ErrorContains(t, err, "exists")
	writeSvcFile(t, src, "nested/alpha/resource.txt", "updated")
	require.NoError(t, os.RemoveAll(filepath.Join(src, "nested/beta")))
	writeSvcFile(t, src, "new/SKILL.md", "---\nname: delta\ndescription: new\n---\nnew")
	opts.Force = true
	r, err = svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Contains(t, r.Skills, "delta")
	require.NoError(t, os.RemoveAll(src))
	for _, target := range targets {
		require.FileExists(t, filepath.Join(target.Path, "beta/SKILL.md"))
		require.FileExists(t, filepath.Join(target.Path, "delta/SKILL.md"))
		data, err := os.ReadFile(filepath.Join(target.Path, "alpha/resource.txt"))
		require.NoError(t, err)
		require.Equal(t, "updated", string(data))
	}
}

func TestDeployPreflightZeroWrites(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *Services, *DeployOptions, string, []common.InstallTarget)
	}{
		{"target-required", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Targets = nil
		}},
		{"unknown-target", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Targets = []string{"missing"}
		}},
		{"unknown-only", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Only = []string{"missing"}
		}},
		{"mutual-exclusion", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.NoRepo = true
		}},
		{"duplicate-name", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			writeSvcFile(t, src, "duplicate/SKILL.md", "---\nname: alpha\ndescription: duplicate\n---\nbody")
		}},
		{"invalid-name", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			writeSvcFile(t, src, "bad/SKILL.md", "---\nname: ../escape\ndescription: bad\n---\nbody")
		}},
		{"symlink-resource", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			require.NoError(t, os.Symlink("resource.txt", filepath.Join(src, "nested/alpha/link")))
		}},
		{"source-overlap", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Into = filepath.Join(src, "library")
		}},
		{"missing-parent", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Into = filepath.Join(t.TempDir(), "absent/library")
		}},
		{"empty-source", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			o.Repo = t.TempDir()
		}},
		{"bare-source", func(t *testing.T, s *Services, o *DeployOptions, src string, ts []common.InstallTarget) {
			writeSvcFile(t, src, "HEAD", "ref")
			require.NoError(t, os.Mkdir(filepath.Join(src, "objects"), 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(src, "refs"), 0o755))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, opts, src, targets := deployFixture(t)
			tc.mutate(t, svc, &opts, src, targets)
			r, err := svc.Deploy(context.Background(), opts)
			require.Error(t, err)
			require.False(t, r.Success)
			require.NotNil(t, r.Error)
			require.Equal(t, "failed", r.Phase)
			require.NoDirExists(t, opts.Into)
			for _, target := range targets {
				require.NoDirExists(t, target.Path)
			}
		})
	}
}

func TestDeployRejectsEveryExistingDestination(t *testing.T) {
	for _, kind := range []string{"empty", "nonempty", "file", "symlink", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			svc, opts, _, targets := deployFixture(t)
			switch kind {
			case "empty":
				require.NoError(t, os.Mkdir(opts.Into, 0o755))
			case "nonempty":
				writeSvcFile(t, opts.Into, "keep.txt", "keep")
			case "file":
				require.NoError(t, os.WriteFile(opts.Into, []byte("keep"), 0o644))
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), opts.Into))
			case "dangling":
				require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "absent"), opts.Into))
			}
			opts.Force = true
			_, err := svc.Deploy(context.Background(), opts)
			require.ErrorContains(t, err, "already exists")
			for _, target := range targets {
				require.NoDirExists(t, target.Path)
			}
		})
	}
}

func TestDeployDryRunAndSelection(t *testing.T) {
	svc, opts, _, targets := deployFixture(t)
	opts.DryRun = true
	opts.Only = []string{"beta"}
	opts.Targets = []string{"one", "one"}
	r, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, "planned", r.Phase)
	require.Len(t, r.Operations, 2)
	require.Equal(t, []string{"one"}, r.Targets)
	require.NoDirExists(t, opts.Into)
	for _, target := range targets {
		require.NoDirExists(t, target.Path)
	}
	opts.DryRun = false
	r, err = svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, []string{"beta"}, r.Skills)
	require.NoDirExists(t, targets[1].Path)
}

func TestDeployAllBuiltinSkillTargetsUseTemporaryPaths(t *testing.T) {
	for _, target := range targetdefs.Builtins(targetdefs.Context{Home: t.TempDir(), Getenv: func(string) string { return "" }}) {
		if !target.AcceptsKind(common.KindSkill) {
			continue
		}
		t.Run(target.Name, func(t *testing.T) {
			for _, direct := range []bool{false, true} {
				_, opts, _, _ := deployFixture(t)
				target.Path = filepath.Join(t.TempDir(), "target")
				svc := newDeploySvc(t, []common.InstallTarget{target})
				opts.Targets = []string{target.Name}
				opts.NoRepo = direct
				if direct {
					opts.Into = ""
				}
				_, err := svc.Deploy(context.Background(), opts)
				require.NoError(t, err)
			}
		})
	}
}

func TestDeployIncompatibleAndOverlappingTargets(t *testing.T) {
	for _, kind := range []string{"command", "plugin", "name", "source", "alias", "nested"} {
		t.Run(kind, func(t *testing.T) {
			_, opts, src, ts := deployFixture(t)
			switch kind {
			case "command":
				ts[0].Accepts = []common.EntryKind{common.KindCommand}
			case "plugin":
				ts[0].Strategies[common.KindSkill] = "plugin:test"
			case "name":
				ts[0].NameRule = common.NameRuleKebabCase
				writeSvcFile(t, src, "bad/SKILL.md", "---\nname: Bad_Name\ndescription: bad\n---\nx")
			case "source":
				ts[0].Path = src
			case "alias":
				ts[1].Path = ts[0].Path
			case "nested":
				ts[1].Path = filepath.Join(ts[0].Path, "nested")
			}
			svc := newDeploySvc(t, ts)
			_, err := svc.Deploy(context.Background(), opts)
			require.Error(t, err)
			require.NoDirExists(t, opts.Into)
		})
	}
}

// A real acquisition boundary with only local fixture bytes; no network tool
// is invoked. Every fetched root is recorded to assert cleanup on all paths.
type deployProvider struct {
	t       *testing.T
	fixture string
	roots   []string
	calls   int
}

func (p *deployProvider) ID() string                         { return "fixture" }
func (p *deployProvider) Label() string                      { return "fixture" }
func (p *deployProvider) Capability() providers.Capability   { return providers.Capability{ID: p.ID()} }
func (p *deployProvider) Normalize(a string) (string, error) { return a, nil }
func (p *deployProvider) CanHandle(a string) bool            { return strings.HasPrefix(a, "fixture://") }
func (p *deployProvider) Fetch(context.Context, string) (string, error) {
	p.calls++
	root := p.t.TempDir()
	p.roots = append(p.roots, root)
	err := filepath.WalkDir(p.fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.fixture, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	return root, err
}

func TestDeployProviderSubpathUpdateAndCleanup(t *testing.T) {
	svc, opts, src, _ := deployFixture(t)
	p := &deployProvider{t: t, fixture: src}
	require.NoError(t, svc.Registry.Register(p))
	opts.Repo = "fixture://collection"
	opts.DryRun = true
	_, err := svc.Deploy(context.Background(), opts)
	require.Error(t, err)
	require.Zero(t, p.calls)
	opts.DryRun = false
	r, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, "cloned", r.Clone)
	require.Equal(t, 1, p.calls)
	require.NoDirExists(t, p.roots[0])
	e := findEntryIn(t, opts.Into, "alpha")
	require.Equal(t, "fixture://collection", e.Origin.Address)
	require.Equal(t, "nested/alpha", e.Origin.Subpath)
	svc.Repo = engines.NewRepository(opts.Into)
	svc.Cfg.Root = opts.Into
	writeSvcFile(t, src, "nested/alpha/resource.txt", "new provider bytes")
	_, err = svc.Update(context.Background(), "alpha", UpdateOptions{})
	require.NoError(t, err)
	require.NoDirExists(t, p.roots[1])
	content, err := os.ReadFile(filepath.Join(e.Path, "resource.txt"))
	require.NoError(t, err)
	require.Equal(t, "new provider bytes", string(content))
	e.Origin.Subpath = "../escape"
	_, _, err = svc.fetchFromOrigin(context.Background(), e)
	require.Error(t, err)
	require.NoDirExists(t, p.roots[2])
	// A single root skill and an old origin without subpath retain the old path.
	e.Origin.Subpath = ""
	path, cleanup, err := svc.fetchFromOrigin(context.Background(), e)
	require.NoError(t, err)
	require.DirExists(t, path)
	cleanup()
	require.NoDirExists(t, path)
}

func TestDeployProviderFailureCleansRoot(t *testing.T) {
	svc, opts, src, _ := deployFixture(t)
	p := &deployProvider{t: t, fixture: src}
	require.NoError(t, svc.Registry.Register(p))
	opts.Repo = "fixture://collection"
	opts.Only = []string{"absent"}
	_, err := svc.Deploy(context.Background(), opts)
	require.Error(t, err)
	require.Equal(t, 1, p.calls)
	require.NoDirExists(t, p.roots[0])
	require.NoDirExists(t, opts.Into)
}

func TestDeployForceReplacesLinkNotReferent(t *testing.T) {
	svc, opts, _, targets := deployFixture(t)
	opts.Into = ""
	opts.NoRepo = true
	opts.Force = true
	opts.Only = []string{"alpha"}
	foreign := t.TempDir()
	writeSvcFile(t, foreign, "keep.txt", "untouched")
	require.NoError(t, os.MkdirAll(targets[0].Path, 0o755))
	require.NoError(t, os.Symlink(foreign, filepath.Join(targets[0].Path, "alpha")))
	_, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(foreign, "keep.txt"))
	require.NoError(t, err)
	require.Equal(t, "untouched", string(data))
	_, err = dal.ReadMeta(filepath.Join(targets[0].Path, "alpha"))
	require.True(t, os.IsNotExist(err))
}

// Cancellation after a real completed slot deterministically injects a later
// failure without replacing the production filesystem or install path.
type cancelAfterSlot struct {
	context.Context
	slot string
}

func (c cancelAfterSlot) Err() error {
	if _, err := os.Lstat(c.slot); err == nil {
		return context.Canceled
	}
	return nil
}

func TestDeployPartialCompletionReport(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "library", true: "direct"}[direct], func(t *testing.T) {
			svc, opts, _, ts := deployFixture(t)
			opts.NoRepo = direct
			if direct {
				opts.Into = ""
			}
			ctx := cancelAfterSlot{Context: context.Background(), slot: filepath.Join(ts[0].Path, "alpha")}
			r, err := svc.Deploy(ctx, opts)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, r.Success)
			require.Len(t, r.Results, 1)
			require.NotEmpty(t, r.Recovery)
			offset := 0
			if !direct {
				offset = 3
				require.DirExists(t, opts.Into)
				for _, op := range r.Operations[:3] {
					require.Equal(t, "completed", op.Status)
				}
			}
			require.Equal(t, "completed", r.Operations[offset].Status)
			require.Equal(t, "failed", r.Operations[offset+1].Status)
			require.Equal(t, "not_run", r.Operations[offset+2].Status)
			require.NoDirExists(t, ts[1].Path)
		})
	}
}

func TestDeployIgnoresArchiveAndCommandsAndStopsAtSkillRoot(t *testing.T) {
	svc, opts, src, _ := deployFixture(t)
	for _, p := range []string{"archived/ignored", "commands/ignored", ".private/ignored", "nested/alpha/examples/ignored"} {
		writeSvcFile(t, src, filepath.Join(p, "SKILL.md"), "---\nname: alpha\ndescription: ignored\n---\nx")
	}
	r, err := svc.Deploy(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, r.Skills, 3)
}
