package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
)

type metaOnlyMigrationItem struct {
	entry *common.Entry
	meta  common.SkillMeta
	path  string
}

func (s *Services) migrateMetaOnly(ctx context.Context, accept bool, expected string, dryRun bool) (*engines.MigrationResult, error) {
	items := []metaOnlyMigrationItem{}
	for _, e := range s.Repo.Scan() {
		if e.Kind != common.KindSkill || e.Status != common.StatusActive {
			continue
		}
		upstream := e.RepositoryPath
		if upstream == "" {
			upstream = e.Path
		}
		legacy, err := dal.ReadSkillMeta(upstream)
		if os.IsNotExist(err) || (err == nil && legacy.SchemaVersion == 3) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("migration_conflict: %s: %w", e.Name, err)
		}
		rel, err := filepath.Rel(s.Cfg.Root, upstream)
		if err != nil {
			return nil, err
		}
		if _, err = os.Lstat(filepath.Join(s.Cfg.Root, dal.SyncForkRel(rel))); err == nil {
			return nil, fmt.Errorf("migration_conflict: unrecorded fork for %s", e.Name)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		meta := common.SkillMeta{Origin: legacy.Origin, SchemaVersion: 3, RevisionKind: "snapshot", Active: "upstream"}
		meta.Path = rel
		meta.Revision = ""
		provider := s.Registry.Get(e.ProviderIDValue())
		if provider == nil {
			provider = s.Registry.Match(legacy.Address)
		}
		if _, ok := provider.(providers.VersionedFetcher); ok {
			match, err := s.matchLegacyBaseline(ctx, e)
			if err != nil {
				return nil, fmt.Errorf("migration_conflict: exact history for %s: %w", e.Name, err)
			}
			if match == nil {
				return nil, fmt.Errorf("migration_conflict: unknown source revision for %s", e.Name)
			}
			meta.Origin = match.Origin
			meta.Path = rel
			meta.RevisionKind = "git"
			match.Cleanup()
		}
		upstreamEntry := *e
		upstreamEntry.Path = upstream
		items = append(items, metaOnlyMigrationItem{entry: &upstreamEntry, meta: meta, path: rel})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].path < items[j].path })
	res := &engines.MigrationResult{Outcome: "planned", Entries: []string{}, ForkMoves: []engines.ForkMove{}, Refresh: []engines.RefreshResult{}, Preview: !accept || dryRun}
	if len(items) == 0 {
		res.Outcome = "current"
		return res, nil
	}
	state := sha256.New()
	for _, item := range items {
		fingerprint, err := dal.TreeFingerprint(item.entry.Path)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(state, "%s:%s:%s\n", item.path, fingerprint, item.meta.Revision)
		res.Entries = append(res.Entries, item.entry.Name)
	}
	ignoreHash, err := dal.TreeFingerprint(filepath.Join(s.Cfg.Root, ".gitignore"))
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(state, ".gitignore:%s\n", ignoreHash)
	res.ExpectedState = hex.EncodeToString(state.Sum(nil))
	if !accept || dryRun {
		return res, nil
	}
	if expected == "" || expected != res.ExpectedState {
		return res, fmt.Errorf("stale_input: migrate --accept requires current preview --expected-state")
	}
	temp := filepath.Join(s.Cfg.Root, ".skm", "temp")
	if err = os.MkdirAll(temp, 0700); err != nil {
		return res, err
	}
	stage, err := os.MkdirTemp(temp, "migrate-meta-")
	if err != nil {
		return res, err
	}
	defer func() {
		_ = os.RemoveAll(stage)
		_ = os.Remove(temp)
		_ = os.Remove(filepath.Join(s.Cfg.Root, ".skm"))
	}()
	moves := make([]dal.SyncReplacement, 0, len(items)+1)
	for _, item := range items {
		candidate := filepath.Join(stage, dal.SyncKey(item.path))
		if err = dal.CopySyncTree(item.entry.Path, candidate); err != nil {
			return res, err
		}
		if err = dal.WriteSkillMeta(candidate, item.meta); err != nil {
			return res, err
		}
		before, err := dal.TreeFingerprint(item.entry.Path)
		if err != nil {
			return res, err
		}
		moves = append(moves, dal.SyncReplacement{Path: item.path, Source: candidate, Expected: before})
	}
	ignore, err := os.ReadFile(filepath.Join(s.Cfg.Root, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}
	if len(ignore) > 0 && ignore[len(ignore)-1] != '\n' {
		ignore = append(ignore, '\n')
	}
	if !containsIgnoreLine(ignore, "/.skm/temp/") {
		ignore = append(ignore, []byte("/.skm/temp/\n")...)
	}
	ignoreStage := filepath.Join(stage, "gitignore")
	if err = os.WriteFile(ignoreStage, ignore, 0600); err != nil {
		return res, err
	}
	moves = append(moves, dal.SyncReplacement{Path: ".gitignore", Source: ignoreStage, Expected: ignoreHash})
	if err = dal.CommitFinalSync(s.Cfg.Root, "migration", moves); err != nil {
		res.Accepted = dal.IsSyncAccepted(err)
		return res, err
	}
	res.Accepted, res.Outcome, res.Preview = true, "migrated", false
	return res, nil
}

func containsIgnoreLine(data []byte, line string) bool {
	for _, item := range strings.Split(string(data), "\n") {
		if item == line {
			return true
		}
	}
	return false
}
