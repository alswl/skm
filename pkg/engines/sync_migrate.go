package engines

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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

// verifiedLegacyGitBaseline promotes a v1 baseline only when the recorded
// repository can locally prove that the exact immutable commit reconstructs
// the old raw tree. A failed proof is intentionally not an error: preserving
// the old tree as a snapshot is the safe migration fallback.
func verifiedLegacyGitBaseline(stage string, origin common.Origin, revision, digest string) (common.BaselineRef, bool) {
	if len(revision) != 40 && len(revision) != 64 || digest == "" {
		return common.BaselineRef{}, false
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return common.BaselineRef{}, false
	}
	repository := origin.RepositoryAddress
	if repository == "" {
		repository = origin.Address
	}
	if repository == "" {
		return common.BaselineRef{}, false
	}
	resolved, err := exec.Command("git", "-C", repository, "rev-parse", "--verify", revision+"^{commit}").Output()
	if err != nil || !strings.EqualFold(strings.TrimSpace(string(resolved)), revision) {
		return common.BaselineRef{}, false
	}
	archiveArgs := []string{"-C", repository, "archive", "--format=tar", revision}
	selected := ""
	if origin.Subpath != "" && origin.Subpath != "." {
		if !filepath.IsLocal(origin.Subpath) {
			return common.BaselineRef{}, false
		}
		selected = filepath.Clean(origin.Subpath)
		archiveArgs = append(archiveArgs, "--", filepath.ToSlash(selected))
	}
	archive, err := exec.Command("git", archiveArgs...).Output()
	if err != nil {
		return common.BaselineRef{}, false
	}
	dir, err := os.MkdirTemp(stage, "verified-git-")
	if err != nil {
		return common.BaselineRef{}, false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		h, nextErr := tr.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return common.BaselineRef{}, false
		}
		// git archive writes a global PAX comment before normal entries.
		// It carries no tree content and must not be extracted as a file.
		if h.Typeflag == tar.TypeXHeader || h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if h.Name == "" || !filepath.IsLocal(filepath.FromSlash(h.Name)) {
			return common.BaselineRef{}, false
		}
		path := filepath.Join(dir, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, os.FileMode(h.Mode)); err != nil {
				return common.BaselineRef{}, false
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return common.BaselineRef{}, false
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode))
			if err != nil {
				return common.BaselineRef{}, false
			}
			_, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil || closeErr != nil {
				return common.BaselineRef{}, false
			}
		default:
			return common.BaselineRef{}, false
		}
	}
	if selected != "" {
		dir = filepath.Join(dir, selected)
	}
	got, err := dal.TreeFingerprint(dir)
	if err != nil || got != digest {
		return common.BaselineRef{}, false
	}
	return common.BaselineRef{Kind: "git", Revision: strings.ToLower(revision), RawDigest: digest}, true
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
		items = append(items, legacyRecord{rel: rec.EntryPath, rec: rec})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].rel < items[j].rel })
	// expected_state authorizes the complete migration scene, not only record
	// metadata. A preview must become stale if a user edits legacy content,
	// ignore rules, or creates a target fork before accepting it.
	state := sha256.New()
	paths := []string{".skm-sync", ".skm-local", "forks", ".gitignore", ".skm"}
	for _, item := range items {
		paths = append(paths, item.rel, dal.SyncForkRel(item.rel))
	}
	for _, path := range paths {
		fingerprint, err := dal.TreeFingerprint(filepath.Join(r.root, path))
		if err != nil {
			return nil, "", err
		}
		if _, err := fmt.Fprintf(state, "%s:%s\n", path, fingerprint); err != nil {
			return nil, "", err
		}
	}
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
	if err = os.MkdirAll(filepath.Join(newState, "baselines"), 0700); err != nil {
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
		upstreamRevision := rec.UpstreamVersion
		if upstreamRevision == "" {
			upstreamRevision = rec.Origin.Revision
		}
		if ref, ok := verifiedLegacyGitBaseline(stage, rec.Origin, upstreamRevision, rec.UpstreamDigest); ok {
			rec.UpstreamBase = ref
		} else {
			if err = stageSnapshot(upstreamBaseline, rec.UpstreamDigest); err != nil {
				return res, err
			}
			rec.UpstreamBase = common.BaselineRef{Kind: "snapshot", Revision: "sha256:" + rec.UpstreamDigest, RawDigest: rec.UpstreamDigest, SnapshotDigest: rec.UpstreamDigest}
		}
		if rec.ForkDigest != "" {
			forkBaseline := upstreamBaseline
			if rec.ForkDigest != rec.UpstreamDigest {
				forkBaseline = filepath.Join(oldState, "fork-baseline")
			}
			forkRevision := rec.ForkVersion
			if forkRevision == "" {
				forkRevision = upstreamRevision
			}
			forkBase, verifiedFork := verifiedLegacyGitBaseline(stage, rec.Origin, forkRevision, rec.ForkDigest)
			if !verifiedFork {
				if err = stageSnapshot(forkBaseline, rec.ForkDigest); err != nil {
					return res, err
				}
				forkBase = common.BaselineRef{Kind: "snapshot", Revision: "sha256:" + rec.ForkDigest, RawDigest: rec.ForkDigest, SnapshotDigest: rec.ForkDigest}
			}
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
	// The ignore file is a transaction target too.  In particular, do not
	// mutate it after old records/forks have been deleted: a crash at that
	// point must still be able to restore the whole v1 repository state.
	ignorePath := filepath.Join(r.root, ".gitignore")
	ignoreBefore, err := dal.TreeFingerprint(ignorePath)
	if err != nil {
		return res, err
	}
	if st, statErr := os.Lstat(ignorePath); statErr == nil && !st.Mode().IsRegular() {
		return res, fmt.Errorf("migration_conflict: .gitignore is not a regular file")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return res, statErr
	}
	ignoreData, readErr := os.ReadFile(ignorePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return res, readErr
	}
	migratedIgnore, _ := dal.MigratedSyncIgnore(ignoreData)
	stagedIgnore := filepath.Join(stage, "gitignore")
	if err = os.WriteFile(stagedIgnore, migratedIgnore, 0644); err != nil {
		return res, err
	}
	replacements = append(replacements,
		dal.SyncReplacement{Path: filepath.Join(".skm", "entries"), Source: filepath.Join(newState, "entries"), Expected: "absent"},
		dal.SyncReplacement{Path: filepath.Join(".skm", "baselines"), Source: filepath.Join(newState, "baselines"), Expected: "absent"},
		dal.SyncReplacement{Path: ".gitignore", Source: stagedIgnore, Expected: ignoreBefore},
		dal.SyncReplacement{Path: ".skm-sync", Expected: oldStateHash},
	)
	if _, err := os.Lstat(filepath.Join(r.root, ".skm-local")); err == nil {
		hash, hashErr := dal.TreeFingerprint(filepath.Join(r.root, ".skm-local"))
		if hashErr != nil {
			return res, hashErr
		}
		replacements = append(replacements, dal.SyncReplacement{Path: ".skm-local", Expected: hash})
	}
	// Migration leaves .gitignore to its transaction replacement so rollback
	// restores the v1 roots and their matching ignore rules together.
	if err = dal.CommitSync(r.root, "migration", replacements); err != nil {
		return res, err
	}
	accepted = true
	if err = dal.InitSyncStore(r.root); err != nil {
		return res, err
	}
	res.Outcome, res.Accepted, res.Preview = "migrated", true, false
	return res, nil
}
