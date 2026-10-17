package engines

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// ForkMergeTimeout (a variable so tests can shorten it) bounds each merge
// command so a stuck tool cannot hang an update.
var ForkMergeTimeout = 30 * time.Second

// ForkMerge is a prepared, not yet applied fork update: the fetched upstream
// and the fork with the user's changes carried onto it. Both live in temp
// directories the caller must Cleanup.
type ForkMerge struct {
	NewUpstream string
	Merged      string
	Before      string // content hash of the current upstream
	After       string // content hash of the new upstream
	Changed     bool
}

// Cleanup removes the temp directories.
func (m *ForkMerge) Cleanup() {
	if m.NewUpstream != "" {
		_ = os.RemoveAll(m.NewUpstream)
	}
	if m.Merged != "" {
		_ = os.RemoveAll(m.Merged)
	}
}

// MergeFork carries the fork's changes (current upstream -> fork) onto the
// fetched upstream: a three-way merge per file with the current upstream as the
// base. Nothing in the repository is touched; any conflict or tool failure
// returns an error and the caller keeps every directory as it was.
//
// The merge is done by `git merge-file`, not diff and patch: when the new
// upstream already contains a change the fork made, patch can silently undo it
// (macOS patch reverses the hunk without a word), while a three-way merge
// treats identical changes on both sides as agreement.
func (r *Repository) MergeFork(ctx context.Context, upstream, fork, fetched string, timeout time.Duration) (*ForkMerge, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("updating a fork needs \"git\" in PATH (it provides the three-way merge)")
	}
	newUp, err := r.stageCopy(fetched)
	if err != nil {
		return nil, err
	}
	m := &ForkMerge{NewUpstream: newUp}
	fail := func(err error) (*ForkMerge, error) { m.Cleanup(); return nil, err }
	if err := dal.RemoveMeta(newUp); err != nil {
		return fail(err)
	}
	if m.Before, err = dal.DirHash(upstream, excludeMeta); err != nil {
		return fail(err)
	}
	if m.After, err = dal.DirHash(newUp, excludeMeta); err != nil {
		return fail(err)
	}
	if m.Before == m.After {
		m.Cleanup()
		return &ForkMerge{Before: m.Before, After: m.After}, nil
	}
	m.Changed = true
	if m.Merged, err = copyDirToTemp(newUp); err != nil {
		return fail(err)
	}
	paths, err := unionFiles(upstream, fork)
	if err != nil {
		return fail(err)
	}
	for _, rel := range paths {
		if err := mergeFile(ctx, rel, filepath.Join(upstream, rel), filepath.Join(fork, rel), filepath.Join(newUp, rel), filepath.Join(m.Merged, rel), timeout); err != nil {
			return fail(err)
		}
	}
	return m, nil
}

// mergeFile settles one path. base is the current upstream, mine the fork,
// theirs the new upstream and out where the result belongs (it starts as a
// copy of theirs).
func mergeFile(ctx context.Context, rel, base, mine, theirs, out string, timeout time.Duration) error {
	if same, err := sameFile(base, mine); err != nil || same {
		return err // the fork did not touch it: the new upstream's version stands
	}
	if same, err := sameFile(mine, theirs); err != nil || same {
		return err // the new upstream already has the fork's version
	}
	if same, err := sameFile(base, theirs); err != nil {
		return err
	} else if same { // only the fork changed it: keep the fork's version
		return copyOrRemove(mine, out)
	}
	for _, p := range []string{base, mine, theirs} {
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("conflict in %s: the fork and the new upstream both changed it", rel)
		}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", "merge-file", "-p", mine, base, theirs)
	var merged, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &merged, &stderr
	err := cmd.Run()
	if cctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("merging %s timed out after %s", rel, timeout)
	}
	if err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return fmt.Errorf("cannot merge %s: %s", rel, msg)
		}
		return fmt.Errorf("conflict in %s: the fork and the new upstream changed the same lines", rel)
	}
	fi, err := os.Stat(mine)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, merged.Bytes(), fi.Mode().Perm()); err != nil {
		return err
	}
	return os.Chmod(out, fi.Mode().Perm()) // WriteFile keeps the mode of an existing file
}

// copyOrRemove makes out match src: a copy of the file or symlink, or absent
// when src does not exist.
func copyOrRemove(src, out string) error {
	fi, err := os.Lstat(src)
	if os.IsNotExist(err) {
		if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(out, data, fi.Mode().Perm())
}

// unionFiles lists the regular files and symlinks, relative to each root, that
// exist in either tree, except the upstream's meta.json.
func unionFiles(a, b string) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range []string{a, b} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || d.IsDir() || rel == "meta.json" {
				return err
			}
			seen[rel] = true
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for rel := range seen {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// sameFile reports whether two paths hold the same content; both missing
// counts as the same. Symlinks are compared by target.
func sameFile(a, b string) (bool, error) {
	ia, errA := os.Lstat(a)
	ib, errB := os.Lstat(b)
	switch {
	case os.IsNotExist(errA) && os.IsNotExist(errB):
		return true, nil
	case os.IsNotExist(errA) || os.IsNotExist(errB):
		return false, nil
	case errA != nil:
		return false, errA
	case errB != nil:
		return false, errB
	}
	linkA, linkB := ia.Mode()&os.ModeSymlink != 0, ib.Mode()&os.ModeSymlink != 0
	if linkA || linkB {
		if !linkA || !linkB {
			return false, nil
		}
		ta, _ := os.Readlink(a)
		tb, _ := os.Readlink(b)
		return ta == tb, nil
	}
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}

// CommitForkUpdate replaces the upstream and fork directories with a prepared
// merge, restoring both if any step fails.
func (r *Repository) CommitForkUpdate(ctx context.Context, entry *common.Entry, m *ForkMerge) error {
	lock, err := dal.AcquireLock(ctx, r.root)
	if err != nil {
		return common.WithExitCode(err, common.ExitError)
	}
	defer lock.Release()

	tx := &dal.FileTransaction{}
	fail := func(err error) error {
		_ = tx.Rollback()
		return common.WithExitCode(err, common.ExitError)
	}
	if err := tx.MoveStage(m.NewUpstream, entry.UpstreamPath); err != nil {
		return fail(err)
	}
	m.NewUpstream = "" // moved; Cleanup must not remove it
	if entry.Origin != nil {
		if err := dal.WriteMeta(entry.UpstreamPath, entry.Origin); err != nil {
			return fail(err)
		}
	}
	if err := tx.MoveStage(m.Merged, entry.Path); err != nil {
		return fail(err)
	}
	m.Merged = ""
	tx.Commit()
	return nil
}
