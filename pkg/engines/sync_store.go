package engines

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func (r *Repository) SyncRecord(e *common.Entry) (*common.SyncRecord, string, error) {
	path := e.RepositoryPath
	if path == "" {
		path = e.Path
	}
	rel, err := filepath.Rel(r.root, path)
	if err != nil {
		return nil, "", err
	}
	rec, err := dal.ReadSyncRecord(r.root, rel)
	return rec, rel, err
}
func (r *Repository) SyncReadCheck() error { return dal.RecoverSync(r.root, true) }
func (r *Repository) SyncRecover() error   { return dal.RecoverSync(r.root, false) }

func (r *Repository) attachSync(e *common.Entry) *common.Entry {
	if e.Kind != common.KindSkill {
		return e
	}
	if err := r.SyncReadCheck(); err != nil {
		return errEntry(e, err.Error())
	}
	rec, _, err := r.SyncRecord(e)
	if os.IsNotExist(err) {
		return e
	}
	if err != nil {
		return errEntry(e, err.Error())
	}
	e.RepositoryPath = e.Path
	e.Origin = &rec.Origin
	if rec.Origin.ProviderID != nil {
		e.ProviderID = rec.Origin.ProviderID
	}
	if rec.ForkPath != "" {
		e.AlternatePath = filepath.Join(r.root, rec.ForkPath)
	}
	if rec.Active == "fork" {
		e.Path = filepath.Join(r.root, rec.ForkPath)
		e.AlternatePath = e.RepositoryPath
		marker := filepath.Join(e.Path, "SKILL.md")
		name, err := frontmatterName(marker)
		if err != nil {
			return errEntry(e, err.Error())
		}
		if name != rec.Name {
			return errEntry(e, "fork identity changed")
		}
		data, err := os.ReadFile(marker)
		if err != nil {
			return errEntry(e, err.Error())
		}
		fm, _, err := dal.ParseFrontmatter(data)
		if err != nil {
			return errEntry(e, err.Error())
		}
		e.Description = fm.Description
		e.Version = fm.Version
	}
	if st, err := r.SyncStatus(e); err == nil {
		e.SyncView = &common.SyncView{Mode: st.Mode, ForkState: st.ForkState, ForkPath: st.ForkPath, UpstreamPath: st.UpstreamPath, BaselineVersion: st.BaselineVersion, Customization: st.Customization, AttemptPath: st.AttemptPath, RollbackAvailable: st.RollbackAvailable}
	}
	return e
}

func (r *Repository) trackedImport(staged, tmp, dest, name string, origin *common.Origin, force bool) error {
	rel, err := filepath.Rel(r.root, dest)
	if err != nil {
		return err
	}
	if _, err = dal.SyncPath(r.root, rel); err != nil {
		return err
	}
	expectedContent, err := dal.TreeFingerprint(dest)
	if err != nil {
		return err
	}
	if expectedContent != "absent" {
		existing := r.findByPath(dest)
		if !force || existing == nil || existing.Origin != nil || existing.RepositoryPath != "" {
			return fmt.Errorf("existing tracked destination: use update or sync baseline; force cannot discard customization")
		}
	}

	if _, err = os.Lstat(filepath.Join(staged, "meta.json")); err == nil {
		return fmt.Errorf("reserved upstream root meta.json; cannot silently discard source data")
	} else if !os.IsNotExist(err) {
		return err
	}
	state, err := os.MkdirTemp("", "skm-state-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(state) }()
	if err = dal.CopySyncTree(staged, filepath.Join(state, "upstream-baseline")); err != nil {
		return err
	}
	raw, err := dal.SyncTree(filepath.Join(state, "upstream-baseline"))
	if err != nil {
		return err
	}
	origin.Path = rel
	version := origin.Revision
	if version == "" {
		version = "sha256:" + raw.Digest
	}
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: name, Generation: 1, Origin: *origin, Active: "upstream", UpstreamVersion: version, UpstreamDigest: raw.Digest}
	if err = dal.WriteSyncJSON(filepath.Join(state, "record.json"), rec); err != nil {
		return err
	}
	stateRel := dal.SyncStateRel(rel)
	expected, err := dal.TreeFingerprint(filepath.Join(r.root, stateRel))
	if err != nil {
		return err
	}
	if expected != "absent" {
		return fmt.Errorf("orphan sync state exists; recover before importing")
	}
	return dal.CommitSync(r.root, rel, []dal.SyncReplacement{{Path: rel, Source: tmp, Expected: expectedContent}, {Path: stateRel, Source: state, Expected: "absent"}}, false)
}
