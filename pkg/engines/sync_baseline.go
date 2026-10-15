package engines

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

func (r *Repository) AssociateBaseline(e *common.Entry, source string, origin *common.Origin, newStart bool) error {
	oldRecord, rel, recordErr := r.SyncRecord(e)
	if err := r.noAttempt(rel); err != nil {
		return err
	}
	if recordErr != nil && !os.IsNotExist(recordErr) {
		return recordErr
	}
	if recordErr == nil && !newStart {
		st, _ := r.SyncStatus(e)
		if st == nil || st.BaselineState == "valid" {
			return fmt.Errorf("association exists")
		}
	}
	state, err := os.MkdirTemp("", "skm-baseline-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(state) }()
	if _, err = os.Lstat(filepath.Join(source, "meta.json")); err == nil {
		return fmt.Errorf("reserved upstream meta.json")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = dal.CopySyncTree(source, filepath.Join(state, "upstream-baseline")); err != nil {
		return err
	}
	if err = dal.CopySyncTree(source, filepath.Join(state, "fork-baseline")); err != nil {
		return err
	}
	m, err := dal.SyncTree(source)
	if err != nil {
		return err
	}
	forkRel := filepath.Join("forks", dal.SyncKey(rel))
	forkPath := filepath.Join(r.root, forkRel)
	forkExpected, err := dal.TreeFingerprint(forkPath)
	if err != nil {
		return err
	}
	if forkExpected != "absent" && (recordErr != nil || oldRecord.Active != "fork") {
		return fmt.Errorf("existing_fork: %s", forkPath)
	}
	local := filepath.Join(state, "local")
	if err = dal.CopySyncTree(e.Path, local); err != nil {
		return err
	}
	// Only recognized legacy management metadata is removed from the maintained copy.
	if e.RepositoryPath == "" && e.Origin != nil {
		if err = os.Remove(filepath.Join(local, "meta.json")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err = r.validateCandidate(local, e.Name); err != nil {
		return err
	}
	community := filepath.Join(state, "community")
	if err = dal.CopySyncTree(source, community); err != nil {
		return err
	}
	if err = normalizeStagedSkill(community, e.Name); err != nil {
		return err
	}
	origin.Path = rel
	rec := common.SyncRecord{Schema: 1, EntryPath: rel, Name: e.Name, Generation: 1, Origin: *origin, Active: "fork", ForkPath: forkRel, UpstreamVersion: origin.Revision, ForkVersion: origin.Revision, UpstreamDigest: m.Digest, ForkDigest: m.Digest}
	if oldRecord != nil {
		rec.Generation = oldRecord.Generation + 1
	}
	if err = dal.WriteSyncJSON(filepath.Join(state, "record.json"), rec); err != nil {
		return err
	}
	// Keep temporary user trees outside the record bundle.
	work, err := os.MkdirTemp("", "skm-baseline-content-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	localDest := filepath.Join(work, "local")
	communityDest := filepath.Join(work, "community")
	if err = os.Rename(local, localDest); err != nil {
		return err
	}
	if err = os.Rename(community, communityDest); err != nil {
		return err
	}
	upstreamExpected, err := dal.TreeFingerprint(filepath.Join(r.root, rel))
	if err != nil {
		return err
	}
	stateExpected, err := dal.TreeFingerprint(filepath.Join(r.root, dal.SyncStateRel(rel)))
	if err != nil {
		return err
	}
	return dal.CommitSync(r.root, rel, []dal.SyncReplacement{{Path: rel, Source: communityDest, Expected: upstreamExpected}, {Path: forkRel, Source: localDest, Expected: forkExpected}, {Path: dal.SyncStateRel(rel), Source: state, Expected: stateExpected}}, false)
}
