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
	e.Origin = &meta.Origin
	if meta.ProviderID != nil {
		e.ProviderID = meta.ProviderID
	}
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
	baselineVersion := meta.Revision
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
			if forkMeta.BaseRevision != "" {
				baselineVersion = forkMeta.BaseRevision
			}
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
	e.SyncView = &common.SyncView{Mode: meta.Active, ForkState: forkState, UpstreamPath: upstream, ForkPath: fork, BaselineVersion: baselineVersion, BaselineState: "fetch_required", Customization: "unknown", RevisionKind: meta.RevisionKind}
	if meta.RevisionKind == "snapshot" {
		e.SyncView.BaselineState = "not_applicable"
		e.SyncView.Customization = "not_applicable"
		e.SyncView.IneligibleReason = "non-Git Skill is a fixed snapshot; import changed source as a new Skill"
	}
	if path, attempt, err := r.finalAttempt(meta.Path); err != nil {
		return errEntry(e, err.Error())
	} else if path != "" {
		e.SyncView.AttemptPath = path
		e.SyncView.OperationID = attempt.OperationID
		e.SyncView.OperationState = "failed_candidate"
		e.SyncView.RollbackAvailable = true
		e.SyncView.NextActions = []string{"skm update " + e.Name + " --accept", "skm update " + e.Name + " --abort"}
	}
	return e
}

func (r *Repository) finalSyncStatus(e *common.Entry, meta *common.SkillMeta) (*SyncStatus, error) {
	upstream := e.RepositoryPath
	if upstream == "" {
		upstream = e.Path
	}
	if meta.SchemaVersion != 3 {
		return nil, fmt.Errorf("migration_required: legacy skill metadata")
	}
	st := &SyncStatus{
		AttemptState: "none", Mode: meta.Active, ForkState: "absent",
		ActivePath: e.Path, UpstreamPath: upstream,
		BaselineVersion: meta.Revision, RevisionKind: meta.RevisionKind, BaselineState: "fetch_required", UpdateEligible: meta.RevisionKind == "git",
		Customization: "unknown", Origin: &meta.Origin, Changes: []SyncChange{},
	}
	if meta.RevisionKind == "snapshot" {
		st.BaselineState = "not_applicable"
		st.Customization = "not_applicable"
		st.IneligibleReason = "non-Git Skill is a fixed snapshot; import changed source as a new Skill"
	}
	fork := filepath.Join(r.root, dal.SyncForkRel(meta.Path))
	if _, err := os.Lstat(fork); err == nil {
		st.ForkPath = fork
		st.ForkState = "retained"
		if meta.Active == "fork" {
			st.ForkState = "active"
			if forkMeta, readErr := dal.ReadForkMeta(fork); readErr == nil && forkMeta.BaseRevision != "" {
				st.BaselineVersion = forkMeta.BaseRevision
				st.ForkBaseVersion = forkMeta.BaseRevision
			}
		} else if forkMeta, readErr := dal.ReadForkMeta(fork); readErr == nil {
			st.ForkBaseVersion = forkMeta.BaseRevision
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	path, attempt, err := r.finalAttempt(meta.Path)
	if err != nil {
		return nil, err
	}
	if path != "" {
		st.AttemptPath = path
		st.AttemptState = "failed"
		st.AttemptVersion = attempt.Version
		st.Failures = attempt.Failures
		st.OperationID = attempt.OperationID
		st.OperationState = "failed_candidate"
		st.RollbackAvailable = true
		st.NextActions = []string{"skm update " + e.Name + " --accept", "skm update " + e.Name + " --abort"}
	}
	return st, nil
}
