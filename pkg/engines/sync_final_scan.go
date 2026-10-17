package engines

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func (r *Repository) attachFinalSkill(e *common.Entry, meta *common.SkillMeta) *common.Entry {
	upstream := e.Path
	rel, err := filepath.Rel(r.root, upstream)
	if err != nil || meta.Path != rel {
		return errEntry(e, fmt.Sprintf("skill metadata path mismatch: %q", meta.Path))
	}
	if meta.ProviderID != nil && e.ProviderID != nil && *meta.ProviderID != *e.ProviderID {
		return errEntry(e, "skill metadata provider mismatch")
	}
	e.Origin = &meta.Origin
	e.RepositoryPath = upstream
	forkRel := dal.SyncForkRel(rel)
	fork := filepath.Join(r.root, forkRel)
	if _, err = os.Lstat(fork); err != nil && !os.IsNotExist(err) {
		return errEntry(e, err.Error())
	}
	forkExists := err == nil
	if meta.Active == "fork" && !forkExists {
		return errEntry(e, "active fork directory missing")
	}
	if forkExists {
		forkMeta, readErr := dal.ReadForkMeta(fork)
		if readErr != nil {
			return errEntry(e, "invalid fork metadata: "+readErr.Error())
		}
		if meta.RevisionKind == "git" && forkMeta.BaseRevision == "" {
			return errEntry(e, "Git fork base revision missing")
		}
		name, nameErr := frontmatterName(filepath.Join(fork, "SKILL.md"))
		if nameErr != nil || name != e.Name {
			return errEntry(e, "fork identity changed")
		}
		e.AlternatePath = fork
		if meta.Active == "fork" {
			e.Path = fork
			e.AlternatePath = upstream
			data, readErr := os.ReadFile(filepath.Join(fork, "SKILL.md"))
			if readErr != nil {
				return errEntry(e, readErr.Error())
			}
			fm, _, parseErr := dal.ParseFrontmatter(data)
			if parseErr != nil {
				return errEntry(e, parseErr.Error())
			}
			e.Description, e.Version = fm.Description, fm.Version
		}
	}
	forkState := "absent"
	if forkExists {
		forkState = "retained"
		if meta.Active == "fork" {
			forkState = "active"
		}
	}
	e.SyncView = &common.SyncView{Mode: "tracked", ForkState: forkState, UpstreamPath: upstream, ForkPath: fork, BaselineVersion: meta.Revision, Customization: "unknown"}
	return e
}
