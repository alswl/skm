package engines

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
)

type finalMigrationItem struct {
	rel  string
	rec  *common.SyncRecord
	meta common.SkillMeta
	fork *common.ForkMeta
}

// PreflightFinalLegacy checks the evidence that a v1-to-v2-to-v3 conversion
// would otherwise discover only after accepting the first phase.
func (r *Repository) PreflightFinalLegacy() error {
	items, _, err := r.legacyRecords()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "skm-legacy-preflight-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for _, item := range items {
		rec := item.rec
		providerID := ""
		if rec.Origin.ProviderID != nil {
			providerID = *rec.Origin.ProviderID
		}
		gitSource := rec.Origin.RepositoryAddress != "" || providerID == "git" || providerID == "github" || providerID == "gitlab" || providerID == "skills-sh"
		if !gitSource {
			continue
		}
		if rec.Origin.RepositoryAddress == "" {
			return fmt.Errorf("migration_conflict: Git repository address is unknown for %s; legacy content was preserved", rec.Name)
		}
		upstreamRev := rec.UpstreamVersion
		if upstreamRev == "" {
			upstreamRev = rec.Origin.Revision
		}
		if _, ok := verifiedLegacyGitBaseline(stage, rec.Origin, upstreamRev, rec.UpstreamDigest); !ok {
			return fmt.Errorf("migration_conflict: unprovable Git revision for %s; legacy content was preserved", rec.Name)
		}
		if rec.ForkPath != "" {
			forkRev := rec.ForkVersion
			if forkRev == "" {
				forkRev = upstreamRev
			}
			if _, ok := verifiedLegacyGitBaseline(stage, rec.Origin, forkRev, rec.ForkDigest); !ok {
				return fmt.Errorf("migration_conflict: unprovable fork base revision for %s; legacy content was preserved", rec.Name)
			}
		}
	}
	return nil
}

// MigrateFinal converts a complete idle v2 store in one recoverable commit.
// Preview reads only; accept binds to fingerprints of every affected tree.
func (r *Repository) MigrateFinal(accept bool, expected string, dryRun bool) (*MigrationResult, error) {
	items, digest, err := r.finalMigrationInventory()
	if err != nil {
		return nil, err
	}
	res := &MigrationResult{Outcome: "planned", ExpectedState: digest, Entries: []string{}, ForkMoves: []ForkMove{}, Refresh: []RefreshResult{}, Preview: !accept || dryRun}
	for _, item := range items {
		res.Entries = append(res.Entries, item.rec.Name)
	}
	if len(items) == 0 && digest == "" {
		res.Outcome = "current"
		return res, nil
	}
	if !accept || dryRun {
		return res, nil
	}
	if expected == "" || expected != digest {
		return res, fmt.Errorf("stale_input: migrate --accept requires current preview --expected-state")
	}
	tempRoot := filepath.Join(r.root, ".skm", "temp")
	if err = os.MkdirAll(tempRoot, 0700); err != nil {
		return res, err
	}
	stage, err := os.MkdirTemp(tempRoot, "migrate-")
	if err != nil {
		return res, err
	}
	defer func() {
		_ = os.RemoveAll(stage)
		_ = os.Remove(tempRoot)
		_ = os.Remove(filepath.Join(r.root, ".skm"))
	}()
	moves := make([]dal.SyncReplacement, 0, len(items)*2+6)
	for _, item := range items {
		upstream := filepath.Join(r.root, item.rel)
		upStage := filepath.Join(stage, dal.SyncKey(item.rel), "upstream")
		if err = dal.CopySyncTree(upstream, upStage); err != nil {
			return res, err
		}
		if err = dal.WriteSkillMeta(upStage, item.meta); err != nil {
			return res, err
		}
		before, err := dal.TreeFingerprint(upstream)
		if err != nil {
			return res, err
		}
		moves = append(moves, dal.SyncReplacement{Path: item.rel, Source: upStage, Expected: before})
		if item.fork != nil {
			forkRel := dal.SyncForkRel(item.rel)
			forkPath := filepath.Join(r.root, forkRel)
			forkStage := filepath.Join(stage, dal.SyncKey(item.rel), "fork")
			if err = dal.CopySyncTree(forkPath, forkStage); err != nil {
				return res, err
			}
			if err = dal.WriteForkMeta(forkStage, *item.fork); err != nil {
				return res, err
			}
			before, err = dal.TreeFingerprint(forkPath)
			if err != nil {
				return res, err
			}
			moves = append(moves, dal.SyncReplacement{Path: forkRel, Source: forkStage, Expected: before})
		}
	}
	// Validate the final scanner in a shadow root before retiring any record.
	shadow := filepath.Join(stage, "shadow")
	for _, move := range moves {
		if err = dal.CopySyncTree(move.Source, filepath.Join(shadow, move.Path)); err != nil {
			return res, err
		}
	}
	shadowEntries := NewRepository(shadow).Scan()
	if len(shadowEntries) != len(items) {
		return res, fmt.Errorf("migration_conflict: final scan found %d entries, expected %d", len(shadowEntries), len(items))
	}
	for _, entry := range shadowEntries {
		if entry.Status != common.StatusActive || entry.SyncView == nil {
			reason := "missing sync view"
			if entry.Error != nil {
				reason = *entry.Error
			}
			return res, fmt.Errorf("migration_conflict: final scan rejected %s: %s", entry.Name, reason)
		}
	}
	for _, rel := range []string{filepath.Join(".skm", "entries"), filepath.Join(".skm", "baselines"), filepath.Join(".skm", "cache"), filepath.Join(".skm", "work"), filepath.Join(".skm", "format.json")} {
		before, err := dal.TreeFingerprint(filepath.Join(r.root, rel))
		if err != nil {
			return res, err
		}
		if before != "absent" {
			moves = append(moves, dal.SyncReplacement{Path: rel, Expected: before})
		}
	}
	ignore, err := migrationFinalIgnore(filepath.Join(r.root, ".gitignore"))
	if err != nil {
		return res, err
	}
	if err = os.WriteFile(filepath.Join(stage, "gitignore"), ignore, 0600); err != nil {
		return res, err
	}
	beforeIgnore, err := dal.TreeFingerprint(filepath.Join(r.root, ".gitignore"))
	if err != nil {
		return res, err
	}
	moves = append(moves, dal.SyncReplacement{Path: ".gitignore", Source: filepath.Join(stage, "gitignore"), Expected: beforeIgnore})
	if err = dal.CommitFinalSync(r.root, "migration", moves); err != nil {
		res.Accepted = dal.IsSyncAccepted(err)
		return res, err
	}
	res.Accepted, res.Outcome, res.Preview = true, "migrated", false
	return res, nil
}

func (r *Repository) finalMigrationInventory() ([]finalMigrationItem, string, error) {
	format := filepath.Join(r.root, ".skm", "format.json")
	var version struct {
		Schema int `json:"schema_version"`
	}
	if err := dal.ReadSyncJSON(format, &version); os.IsNotExist(err) {
		return nil, "", nil
	} else if err != nil || version.Schema != 2 {
		return nil, "", fmt.Errorf("migration_conflict: invalid v2 format")
	}
	if err := dal.RecoverSync(r.root, true); err != nil {
		return nil, "", err
	}
	for _, rel := range []string{filepath.Join(".skm", "work", "operations"), filepath.Join(".skm", "work", "pending")} {
		pending, pendingErr := os.ReadDir(filepath.Join(r.root, rel))
		if pendingErr != nil && !os.IsNotExist(pendingErr) {
			return nil, "", pendingErr
		}
		if len(pending) > 0 {
			return nil, "", fmt.Errorf("migration_conflict: recover pending old operation in %s first", rel)
		}
	}
	entriesRoot := filepath.Join(r.root, ".skm", "entries")
	dirs, err := os.ReadDir(entriesRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, "", err
	}
	items := make([]finalMigrationItem, 0, len(dirs))
	for _, d := range dirs {
		if !d.IsDir() {
			return nil, "", fmt.Errorf("migration_conflict: invalid v2 entry %s", d.Name())
		}
		var raw common.SyncRecord
		if err = dal.ReadSyncJSON(filepath.Join(entriesRoot, d.Name(), "record.json"), &raw); err != nil {
			return nil, "", err
		}
		rec, err := dal.ReadSyncRecord(r.root, raw.EntryPath)
		if err != nil || dal.SyncKey(raw.EntryPath) != d.Name() {
			return nil, "", fmt.Errorf("migration_conflict: invalid v2 record %s", d.Name())
		}
		if rec.Operation != nil {
			return nil, "", fmt.Errorf("migration_conflict: recover pending operation for %s first", rec.Name)
		}
		if _, err = os.Lstat(filepath.Join(r.root, rec.EntryPath, "SKILL.md")); err != nil {
			return nil, "", fmt.Errorf("migration_conflict: missing skill %s: %w", rec.EntryPath, err)
		}
		meta := common.SkillMeta{Origin: rec.Origin, SchemaVersion: 3, RevisionKind: "snapshot", Active: rec.Active}
		meta.Path = rec.EntryPath
		meta.Revision = ""
		providerID := ""
		if rec.Origin.ProviderID != nil {
			providerID = *rec.Origin.ProviderID
		}
		gitSource := rec.Origin.RepositoryAddress != "" || providerID == "git" || providerID == "github" || providerID == "gitlab" || providerID == "skills-sh"
		if gitSource {
			if rec.Origin.RepositoryAddress == "" || rec.UpstreamBase.Kind != "git" || rec.UpstreamBase.Revision == "" {
				return nil, "", fmt.Errorf("migration_conflict: unprovable Git revision for %s", rec.Name)
			}
			meta.RevisionKind = "git"
			meta.Revision = rec.UpstreamBase.Revision
		}
		var fork *common.ForkMeta
		if rec.ForkPath != "" {
			if rec.ForkPath != dal.SyncForkRel(rec.EntryPath) {
				return nil, "", fmt.Errorf("migration_conflict: nonstandard fork path for %s", rec.Name)
			}
			if _, err = os.Lstat(filepath.Join(r.root, rec.ForkPath, "SKILL.md")); err != nil {
				return nil, "", fmt.Errorf("migration_conflict: missing fork for %s: %w", rec.Name, err)
			}
			fork = &common.ForkMeta{SchemaVersion: 3}
			if meta.RevisionKind == "git" {
				if rec.ForkBase == nil || rec.ForkBase.Kind != "git" {
					return nil, "", fmt.Errorf("migration_conflict: unknown fork base revision for %s", rec.Name)
				}
				fork.BaseRevision = rec.ForkBase.Revision
			}
		}
		items = append(items, finalMigrationItem{rel: rec.EntryPath, rec: rec, meta: meta, fork: fork})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].rel < items[j].rel })
	state := sha256.New()
	paths := []string{filepath.Join(".skm", "entries"), filepath.Join(".skm", "baselines"), filepath.Join(".skm", "cache"), filepath.Join(".skm", "work"), filepath.Join(".skm", "format.json"), ".gitignore"}
	for _, item := range items {
		paths = append(paths, item.rel)
		if item.fork != nil {
			paths = append(paths, dal.SyncForkRel(item.rel))
		}
	}
	for _, path := range paths {
		fingerprint, err := dal.TreeFingerprint(filepath.Join(r.root, path))
		if err != nil {
			return nil, "", err
		}
		_, _ = fmt.Fprintf(state, "%s:%s\n", path, fingerprint)
	}
	return items, hex.EncodeToString(state.Sum(nil)), nil
}

func migrationFinalIgnore(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	remove := map[string]bool{"!/.skm/": true, "!/.skm/format.json": true, "!/.skm/entries/": true, "!/.skm/entries/**": true, "/.skm/cache/": true, "/.skm/work/": true}
	var lines []string
	content := strings.TrimSuffix(string(data), "\n")
	if content != "" {
		for _, line := range strings.Split(content, "\n") {
			if remove[line] || line == "/.skm/temp/" {
				continue
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "/.skm/temp/")
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}
