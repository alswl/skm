package engines

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

// MigrationResult is deliberately small: callers first bind an accept request
// to ExpectedState returned by preview, then the repository re-checks it while
// holding the lock.
type MigrationResult struct {
	Outcome       string          `json:"outcome"`
	ExpectedState string          `json:"expected_state,omitempty"`
	Entries       []string        `json:"entries"`
	ForkMoves     []ForkMove      `json:"fork_moves,omitempty"`
	Refresh       []RefreshResult `json:"refresh,omitempty"`
	Accepted      bool            `json:"accepted"`
	Preview       bool            `json:"preview"`
}

type ForkMove struct {
	Entry, OldPath, NewPath string
}

type legacyRecord struct {
	rel string
	rec common.SyncRecord
}

func (r *Repository) legacyRecords() ([]legacyRecord, string, error) {
	legacyRoot := filepath.Join(r.root, ".skm-sync")
	root := filepath.Join(legacyRoot, "entries")
	dirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var format struct {
		Schema int `json:"schema_version"`
	}
	if err := dal.ReadSyncJSON(filepath.Join(legacyRoot, "format.json"), &format); err != nil || format.Schema != 1 {
		if err != nil {
			return nil, "", fmt.Errorf("migration_conflict: invalid legacy format: %w", err)
		}
		return nil, "", fmt.Errorf("migration_conflict: unsupported legacy format")
	}
	items := make([]legacyRecord, 0, len(dirs))
	state := sha256.New()
	for _, d := range dirs {
		if !d.IsDir() {
			return nil, "", fmt.Errorf("migration_conflict: invalid legacy entry %s", d.Name())
		}
		var rec common.SyncRecord
		if err := dal.ReadSyncJSON(filepath.Join(root, d.Name(), "record.json"), &rec); err != nil {
			return nil, "", err
		}
		if rec.Schema != 1 || rec.EntryPath == "" || rec.Name == "" || (rec.Active != "upstream" && rec.Active != "fork") {
			return nil, "", fmt.Errorf("migration_conflict: corrupt legacy record %s", d.Name())
		}
		if dal.SyncKey(rec.EntryPath) != d.Name() {
			return nil, "", fmt.Errorf("migration_conflict: record key mismatch for %s", rec.EntryPath)
		}
		if _, err := dal.SyncPath(r.root, rec.EntryPath); err != nil {
			return nil, "", err
		}
		if _, err := fmt.Fprintf(state, "%s:%s:%s:%d", d.Name(), rec.UpstreamDigest, rec.ForkDigest, rec.Generation); err != nil {
			return nil, "", err
		}
		items = append(items, legacyRecord{rel: rec.EntryPath, rec: rec})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].rel < items[j].rel })
	return items, hex.EncodeToString(state.Sum(nil)), nil
}

// MigrateLegacy converts only a complete idle v1 store. Pending v1 work is
// intentionally rejected because v2 must not guess whether it represented a
// user decision or an interrupted replacement.
func (r *Repository) MigrateLegacy(accept bool, expected string, dryRun bool) (*MigrationResult, error) {
	items, digest, err := r.legacyRecords()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if dal.LegacySyncStore(r.root) {
			return nil, fmt.Errorf("migration_conflict: legacy state has no readable records")
		}
		return &MigrationResult{Outcome: "current", Entries: []string{}, Preview: dryRun}, nil
	}
	res := &MigrationResult{Outcome: "planned", ExpectedState: digest, Entries: make([]string, 0, len(items)), ForkMoves: []ForkMove{}, Refresh: []RefreshResult{}, Preview: !accept || dryRun}
	for _, item := range items {
		res.Entries = append(res.Entries, item.rec.Name)
	}
	if !accept || dryRun {
		return res, nil
	}
	if expected == "" || expected != digest {
		return res, fmt.Errorf("stale_input: migrate --accept requires preview --expected-state")
	}
	if _, err := os.Lstat(filepath.Join(r.root, ".skm")); err == nil {
		return res, fmt.Errorf("migration_conflict: .skm already exists")
	} else if !os.IsNotExist(err) {
		return res, err
	}
	if _, err := os.Lstat(filepath.Join(r.root, ".skm-local", "pending")); err == nil {
		return res, fmt.Errorf("migration_conflict: resolve legacy pending updates before migration")
	} else if !os.IsNotExist(err) {
		return res, err
	}
	if _, err := os.Lstat(filepath.Join(r.root, ".skm-local", "transactions")); err == nil {
		return res, fmt.Errorf("migration_conflict: resolve legacy transactions before migration")
	} else if !os.IsNotExist(err) {
		return res, err
	}

	stage, err := os.MkdirTemp("", "skm-migrate-")
	if err != nil {
		return res, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	newState := filepath.Join(stage, ".skm")
	if err = os.MkdirAll(filepath.Join(newState, "entries"), 0700); err != nil {
		return res, err
	}
	if err = dal.WriteSyncJSON(filepath.Join(newState, "format.json"), map[string]int{"schema_version": 2}); err != nil {
		return res, err
	}
	replacements := []dal.SyncReplacement{}
	snapshots := map[string]bool{}
	stageSnapshot := func(source, digest string) error {
		if digest == "" {
			return fmt.Errorf("migration_conflict: missing legacy baseline digest")
		}
		got, err := dal.TreeFingerprint(source)
		if err != nil || got != digest {
			if err != nil {
				return err
			}
			return fmt.Errorf("migration_conflict: legacy baseline digest mismatch for %s", source)
		}
		if snapshots[digest] {
			return nil
		}
		if err := dal.CopySyncTree(source, filepath.Join(newState, "baselines", digest)); err != nil {
			return err
		}
		snapshots[digest] = true
		return nil
	}
	for _, item := range items {
		oldState := filepath.Join(r.root, ".skm-sync", "entries", dal.SyncKey(item.rel))
		newEntry := filepath.Join(newState, "entries", dal.SyncKey(item.rel))
		if err = dal.CopySyncTree(oldState, newEntry); err != nil {
			return res, err
		}
		upstreamBaseline := filepath.Join(oldState, "upstream-baseline")
		if err = stageSnapshot(upstreamBaseline, item.rec.UpstreamDigest); err != nil {
			return res, err
		}
		// A v1 record must not bless a directly edited upstream directory as a
		// clean v2 upstream. Rebuild the maintained form from its recorded raw
		// baseline and compare it before scheduling any replacement.
		expectedUpstream := filepath.Join(stage, "expected-upstream-"+dal.SyncKey(item.rel))
		if err = dal.CopySyncTree(upstreamBaseline, expectedUpstream); err != nil {
			return res, err
		}
		if err = normalizeStagedSkill(expectedUpstream, item.rec.Name); err != nil {
			return res, fmt.Errorf("migration_conflict: cannot normalize legacy upstream %s: %w", item.rel, err)
		}
		expectedContent, err := dal.TreeFingerprint(expectedUpstream)
		if err != nil {
			return res, err
		}
		actualContent, err := dal.TreeFingerprint(filepath.Join(r.root, item.rel))
		if err != nil {
			return res, err
		}
		if actualContent != expectedContent {
			return res, fmt.Errorf("migration_conflict: upstream content changed for %s", item.rel)
		}
		rec := item.rec
		rec.Schema = 2
		rec.UpstreamBase = common.BaselineRef{Kind: "snapshot", Revision: "sha256:" + rec.UpstreamDigest, RawDigest: rec.UpstreamDigest, SnapshotDigest: rec.UpstreamDigest}
		if rec.ForkDigest != "" {
			forkBaseline := upstreamBaseline
			if rec.ForkDigest != rec.UpstreamDigest {
				forkBaseline = filepath.Join(oldState, "fork-baseline")
			}
			if err = stageSnapshot(forkBaseline, rec.ForkDigest); err != nil {
				return res, err
			}
			forkBase := common.BaselineRef{Kind: "snapshot", Revision: "sha256:" + rec.ForkDigest, RawDigest: rec.ForkDigest, SnapshotDigest: rec.ForkDigest}
			rec.ForkBase = &forkBase
		}
		// Version 2 resolves baseline snapshots by digest from .skm/baselines.
		// Do not carry v1's per-entry snapshots as a second source of truth.
		if err = os.RemoveAll(filepath.Join(newEntry, "upstream-baseline")); err != nil {
			return res, err
		}
		if err = os.RemoveAll(filepath.Join(newEntry, "fork-baseline")); err != nil {
			return res, err
		}
		if rec.ForkPath != "" {
			oldFork := rec.ForkPath
			rec.ForkPath = dal.SyncForkRel(item.rel)
			oldHash, hashErr := dal.TreeFingerprint(filepath.Join(r.root, oldFork))
			if hashErr != nil || oldHash == "absent" {
				return res, fmt.Errorf("migration_conflict: missing legacy fork %s", oldFork)
			}
			if _, statErr := os.Lstat(filepath.Join(r.root, rec.ForkPath)); statErr == nil {
				return res, fmt.Errorf("migration_conflict: target fork exists %s", rec.ForkPath)
			} else if !os.IsNotExist(statErr) {
				return res, statErr
			}
			rec.ForkContentDigest = oldHash
			res.ForkMoves = append(res.ForkMoves, ForkMove{Entry: rec.Name, OldPath: oldFork, NewPath: rec.ForkPath})
			replacements = append(replacements, dal.SyncReplacement{Path: rec.ForkPath, Source: filepath.Join(r.root, oldFork), Expected: "absent"}, dal.SyncReplacement{Path: oldFork, Expected: oldHash})
		}
		upHash, hashErr := dal.TreeFingerprint(filepath.Join(r.root, item.rel))
		if hashErr != nil {
			return res, hashErr
		}
		rec.UpstreamContentDigest = upHash
		if err = dal.WriteSyncJSON(filepath.Join(newEntry, "record.json"), rec); err != nil {
			return res, err
		}
	}
	// CommitSync needs the v2 root to host its journal. This is a bootstrap, not
	// an accepted state: every failure before CommitSync succeeds removes it.
	if err = os.Mkdir(filepath.Join(r.root, ".skm"), 0700); err != nil {
		return res, err
	}
	if err = dal.WriteSyncJSON(filepath.Join(r.root, ".skm", "format.json"), map[string]int{"schema_version": 2}); err != nil {
		_ = dal.CleanupMigrationBootstrap(r.root)
		return res, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = dal.CleanupMigrationBootstrap(r.root)
		}
	}()
	oldStateHash, err := dal.TreeFingerprint(filepath.Join(r.root, ".skm-sync"))
	if err != nil {
		return res, err
	}
	replacements = append(replacements,
		dal.SyncReplacement{Path: filepath.Join(".skm", "entries"), Source: filepath.Join(newState, "entries"), Expected: "absent"},
		dal.SyncReplacement{Path: filepath.Join(".skm", "baselines"), Source: filepath.Join(newState, "baselines"), Expected: "absent"},
		dal.SyncReplacement{Path: ".skm-sync", Expected: oldStateHash},
	)
	if _, err := os.Lstat(filepath.Join(r.root, ".skm-local")); err == nil {
		hash, hashErr := dal.TreeFingerprint(filepath.Join(r.root, ".skm-local"))
		if hashErr != nil {
			return res, hashErr
		}
		replacements = append(replacements, dal.SyncReplacement{Path: ".skm-local", Expected: hash})
	}
	// CommitSync can initialise the ignore entries only after the old roots have
	// moved. Pre-create the v2 root in the replacement source, then append the
	// exact rules after commit below.
	if err = dal.CommitSync(r.root, "migration", replacements, false); err != nil {
		return res, err
	}
	accepted = true
	if err = dal.InitSyncStore(r.root); err != nil {
		return res, err
	}
	if err = dal.RemoveLegacySyncIgnore(r.root); err != nil {
		return res, err
	}
	res.Outcome, res.Accepted, res.Preview = "migrated", true, false
	return res, nil
}
