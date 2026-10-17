package engines

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// ForkDir is the top-level directory holding forks. A fork of
// skills/<path> lives at skills-fork/<path>; its existence is the only record
// that the Skill uses a fork.
const ForkDir = "skills-fork"

// ForkPath returns the fork location paired with an upstream skills/ directory.
func (r *Repository) ForkPath(upstream string) (string, error) {
	rel, err := filepath.Rel(filepath.Join(r.root, "skills"), upstream)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%q is not under skills/", upstream)
	}
	return filepath.Join(r.root, ForkDir, rel), nil
}

// UpstreamOf returns the skills/ directory of an entry, whether or not it
// currently selects a fork.
func UpstreamOf(e *common.Entry) string {
	if e.UpstreamPath != "" {
		return e.UpstreamPath
	}
	return e.Path
}

// attachForks folds each skills-fork/<path> directory into the logical entry
// of its upstream: the fork becomes the active Path and the upstream stays
// reachable as UpstreamPath. A fork whose marker is broken makes the entry an
// error rather than silently falling back to the upstream.
func (r *Repository) attachForks(entries []*common.Entry) []*common.Entry {
	for i, up := range entries {
		if up.Kind != common.KindSkill || up.Status != common.StatusActive {
			continue
		}
		fork, err := r.ForkPath(up.Path)
		if err != nil || !dal.IsDir(fork) {
			continue
		}
		group := ""
		if up.Group != nil {
			group = *up.Group
		}
		fe := r.buildDirEntry(fork, *up.ProviderID, group, common.KindSkill, common.StatusActive)
		if fe.Status != common.StatusActive {
			entries[i] = errEntry(up, "fork: "+*fe.Error+"; fix skills-fork/ or run fork --replace/--discard")
			continue
		}
		if fe.Name != up.Name {
			entries[i] = errEntry(up, fmt.Sprintf("fork: name %q differs from upstream %q", fe.Name, up.Name))
			continue
		}
		fe.Origin, fe.UpstreamPath = up.Origin, up.Path
		entries[i] = fe
	}
	return entries
}

// StageFork copies the upstream directory to a temp directory ready to be
// moved into skills-fork. The root meta.json belongs to the upstream and is
// not part of a fork.
func (r *Repository) StageFork(upstream string) (string, error) {
	tmp, err := copyDirToTemp(upstream)
	if err != nil {
		return "", err
	}
	if err := dal.RemoveMeta(tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}
