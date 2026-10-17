package services

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/stretchr/testify/require"
)

type forkFixture struct {
	svc    *Services
	root   string
	src    string // the upstream source the Skill was imported from
	target string
}

// newForkFixture imports a three-file local Skill and installs it to a target.
func newForkFixture(t *testing.T) *forkFixture {
	t.Helper()
	root, src, target := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	writeFile(t, src, "SKILL.md", frontmatter("review", "Review"))
	writeFile(t, src, "a.txt", numbered("a", 12))
	writeFile(t, src, "b.txt", numbered("b", 12))
	svc, err := New(newCfg(root, []common.InstallTarget{skillTarget("t", target)}), common.NewLogger(false))
	require.NoError(t, err)
	_, err = svc.Import(context.Background(), src, ImportOptions{})
	require.NoError(t, err)
	_, err = svc.Install(context.Background(), "review", InstallOptions{Targets: []string{"t"}})
	require.NoError(t, err)
	return &forkFixture{svc: svc, root: root, src: src, target: target}
}

func numbered(prefix string, n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString(prefix + string(rune('0'+i%10)) + "-" + string(rune('a'+i)) + "\n")
	}
	return b.String()
}

func (f *forkFixture) upstream() string { return filepath.Join(f.root, "skills", "local", "review") }
func (f *forkFixture) fork() string     { return filepath.Join(f.root, "skills-fork", "local", "review") }

func (f *forkFixture) edit(t *testing.T, dir, name, from, to string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	require.Contains(t, string(data), from)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(strings.Replace(string(data), from, to, 1)), 0o644))
}

func (f *forkFixture) read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(data)
}

func (f *forkFixture) update(t *testing.T, opts UpdateOptions) (*engines.UpdateResult, error) {
	t.Helper()
	return f.svc.Update(context.Background(), "review", opts)
}

func (f *forkFixture) linkTarget(t *testing.T) string {
	t.Helper()
	dest, err := os.Readlink(filepath.Join(f.target, "review"))
	require.NoError(t, err)
	return dest
}

// snapshot records every file and symlink under dir and the repository root's
// own entries, so a test can prove nothing changed or was left behind.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			link, _ := os.Readlink(path)
			out[rel] = "-> " + link
		case d.IsDir():
			out[rel] = "dir"
		default:
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			out[rel] = string(data)
		}
		return nil
	}))
	return out
}

func TestForkCreateReplaceDiscard(t *testing.T) {
	f := newForkFixture(t)
	require.Equal(t, f.upstream(), f.linkTarget(t))

	res, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	require.Equal(t, "forked", res.Action)
	require.Equal(t, f.fork(), res.ActivePath)
	require.Equal(t, f.fork(), f.linkTarget(t), "installs follow the fork")
	require.NoFileExists(t, filepath.Join(f.fork(), "meta.json"), "a fork carries no management file")
	require.NoDirExists(t, filepath.Join(f.root, ".skm"))
	entry, err := f.svc.ResolveEntry("review")
	require.NoError(t, err)
	require.Equal(t, f.fork(), entry.Path)
	require.Equal(t, f.upstream(), entry.UpstreamPath)
	require.Len(t, f.svc.Scan(), 1, "upstream and fork are one logical Skill")

	_, err = f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.ErrorContains(t, err, "--replace")

	f.edit(t, f.fork(), "a.txt", "a1-b", "mine")
	res, err = f.svc.Fork(context.Background(), "review", ForkOptions{Replace: true})
	require.NoError(t, err)
	require.Equal(t, "replaced", res.Action)
	require.NotContains(t, f.read(t, f.fork(), "a.txt"), "mine")
	require.Equal(t, f.fork(), f.linkTarget(t))

	res, err = f.svc.Fork(context.Background(), "review", ForkOptions{Discard: true})
	require.NoError(t, err)
	require.Equal(t, "discarded", res.Action)
	require.NoDirExists(t, f.fork())
	require.Equal(t, f.upstream(), f.linkTarget(t))
	_, err = f.svc.Fork(context.Background(), "review", ForkOptions{Discard: true})
	require.ErrorContains(t, err, "no fork")
	_, err = f.svc.Fork(context.Background(), "review", ForkOptions{Replace: true, Discard: true})
	require.Error(t, err)
}

func TestUpdateWithoutForkReplacesUpstream(t *testing.T) {
	f := newForkFixture(t)
	f.edit(t, f.src, "a.txt", "a1-b", "upstream")
	res, err := f.update(t, UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Contains(t, f.read(t, f.upstream(), "a.txt"), "upstream")
	require.NoDirExists(t, f.fork())
}

func TestUpdateWithForkCarriesChangesAcrossUpdates(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "b.txt", "b1-b", "mine")
	writeFile(t, f.fork(), "notes.txt", "my notes\n")

	f.edit(t, f.src, "a.txt", "a1-b", "upstream-1")
	res, err := f.update(t, UpdateOptions{})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Contains(t, f.read(t, f.upstream(), "a.txt"), "upstream-1")
	require.Contains(t, f.read(t, f.fork(), "a.txt"), "upstream-1")
	require.Contains(t, f.read(t, f.fork(), "b.txt"), "mine")
	require.Equal(t, "my notes\n", f.read(t, f.fork(), "notes.txt"))
	require.NotContains(t, f.read(t, f.upstream(), "b.txt"), "mine", "the upstream directory stays pristine")
	require.NoFileExists(t, filepath.Join(f.upstream(), "notes.txt"))

	// A second update builds on the first one.
	f.edit(t, f.src, "a.txt", "a2-c", "upstream-2")
	writeFile(t, f.src, "extra/new.txt", "new upstream file\n")
	_, err = f.update(t, UpdateOptions{})
	require.NoError(t, err)
	require.Contains(t, f.read(t, f.fork(), "a.txt"), "upstream-1")
	require.Contains(t, f.read(t, f.fork(), "a.txt"), "upstream-2")
	require.Contains(t, f.read(t, f.fork(), "b.txt"), "mine")
	require.Equal(t, "new upstream file\n", f.read(t, f.fork(), "extra/new.txt"))
	require.Equal(t, f.fork(), f.linkTarget(t))
	require.NoDirExists(t, filepath.Join(f.root, ".skm"))

	res, err = f.update(t, UpdateOptions{})
	require.NoError(t, err)
	require.False(t, res.Changed, "nothing new upstream")
}

func TestUpdateWithForkDoesNotDuplicateChangeUpstreamAlreadyHas(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "a.txt", "a1-b", "same-edit")
	f.edit(t, f.fork(), "b.txt", "b1-b", "only-mine")
	f.edit(t, f.src, "a.txt", "a1-b", "same-edit") // upstream adopts the fork's change
	f.edit(t, f.src, "a.txt", "a5-f", "upstream-extra")

	_, err = f.update(t, UpdateOptions{})
	require.NoError(t, err)
	got := f.read(t, f.fork(), "a.txt")
	require.Equal(t, 1, strings.Count(got, "same-edit"))
	require.Contains(t, got, "upstream-extra")
	require.Contains(t, f.read(t, f.fork(), "b.txt"), "only-mine")
}

func TestUpdateWithForkConflictChangesNothing(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "a.txt", "a1-b", "mine")
	f.edit(t, f.src, "a.txt", "a1-b", "theirs")
	before := snapshot(t, f.root)
	beforeTarget := snapshot(t, f.target)

	_, err = f.update(t, UpdateOptions{})
	require.ErrorContains(t, err, "conflict in a.txt")
	require.ErrorContains(t, err, "was not changed")
	require.Equal(t, before, snapshot(t, f.root), "repository is byte-identical and has no leftovers")
	require.Equal(t, beforeTarget, snapshot(t, f.target))
}

func TestUpdateWithForkBinaryChangeStops(t *testing.T) {
	f := newForkFixture(t)
	writeFile(t, f.src, "blob.bin", "v1\x00\x01")
	_, err := f.update(t, UpdateOptions{})
	require.NoError(t, err)
	_, err = f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	writeFile(t, f.fork(), "blob.bin", "mine\x00\x02")
	writeFile(t, f.src, "blob.bin", "theirs\x00\x03")
	before := snapshot(t, f.root)
	_, err = f.update(t, UpdateOptions{})
	require.ErrorContains(t, err, "binary")
	require.Equal(t, before, snapshot(t, f.root))
}

func TestUpdateWithForkDryRunChangesNothing(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "b.txt", "b1-b", "mine")
	f.edit(t, f.src, "a.txt", "a1-b", "upstream")
	before := snapshot(t, f.root)
	res, err := f.update(t, UpdateOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, before, snapshot(t, f.root))

	f.edit(t, f.fork(), "a.txt", "a1-b", "mine")
	_, err = f.update(t, UpdateOptions{DryRun: true})
	require.Error(t, err, "dry-run reports a conflict too")
}

func TestUpdateWithForkNeedsGit(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.src, "a.txt", "a1-b", "upstream")
	before := snapshot(t, f.root)
	t.Setenv("PATH", t.TempDir())
	_, err = f.update(t, UpdateOptions{})
	require.ErrorContains(t, err, `needs "git"`)
	require.Equal(t, before, snapshot(t, f.root))
}

func TestUpdateWithForkStopsWhenMergeHangs(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "a.txt", "a1-b", "mine") // both sides touch a.txt: a merge is needed
	f.edit(t, f.src, "a.txt", "a9-j", "upstream")
	before := snapshot(t, f.root)

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := engines.ForkMergeTimeout
	engines.ForkMergeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { engines.ForkMergeTimeout = old })

	started := time.Now()
	_, err = f.update(t, UpdateOptions{})
	require.ErrorContains(t, err, "timed out")
	require.Less(t, time.Since(started), 10*time.Second)
	require.Equal(t, before, snapshot(t, f.root))
}

func TestForkedSkillRefusesDirectoryMovingOperations(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	before := snapshot(t, f.root)
	_, err = f.svc.Archive(context.Background(), "review", engines.LifecycleOptions{})
	require.ErrorContains(t, err, "--discard")
	_, err = f.svc.Delete(context.Background(), "review", engines.LifecycleOptions{Force: true})
	require.ErrorContains(t, err, "--discard")
	require.Equal(t, before, snapshot(t, f.root))
}

func TestBatchUpdateReportsForkConflictAndContinues(t *testing.T) {
	f := newForkFixture(t)
	_, err := f.svc.Fork(context.Background(), "review", ForkOptions{})
	require.NoError(t, err)
	f.edit(t, f.fork(), "a.txt", "a1-b", "mine")
	f.edit(t, f.src, "a.txt", "a1-b", "theirs")
	res := f.svc.BatchUpdate(context.Background(), false)
	require.Len(t, res.Failed, 1)
	require.Equal(t, "review", res.Failed[0].Name)
	require.Contains(t, res.Failed[0].Reason, "conflict in a.txt")
}
