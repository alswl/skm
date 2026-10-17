package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/engines"
	"github.com/alswl/skm/skm/pkg/providers"
)

func (s *Services) finalSyncDiff(ctx context.Context, e *common.Entry, meta *common.SkillMeta) (string, error) {
	if meta.RevisionKind != "git" {
		return "", fmt.Errorf("not_applicable: non-Git Skill has no historical customization diff")
	}
	fork := filepath.Join(s.Cfg.Root, dal.SyncForkRel(meta.Path))
	if _, err := os.Lstat(fork); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	forkMeta, err := dal.ReadForkMeta(fork)
	if err != nil || forkMeta.BaseRevision == "" {
		return "", fmt.Errorf("migration_required: fork has no exact Git base revision")
	}
	provider := s.Registry.Get(e.ProviderIDValue())
	versioned, ok := provider.(providers.VersionedFetcher)
	if !ok {
		return "", fmt.Errorf("historical_source_unavailable: provider cannot fetch exact Git base")
	}
	got, err := versioned.FetchVersioned(ctx, providers.FetchRequest{
		Address: meta.Address, RepositoryAddress: meta.RepositoryAddress,
		RequestedRef: meta.RequestedRef, Revision: forkMeta.BaseRevision, Subpath: meta.Subpath,
	})
	if err != nil {
		return "", fmt.Errorf("historical_source_unavailable: %w", err)
	}
	defer fetchCleanup(provider, got.Path)()
	base := filepath.Join(got.Path, got.ResolvedSubpath)
	bm, err := dal.PayloadSyncTree(base)
	if err != nil {
		return "", err
	}
	lm, err := dal.PayloadSyncTree(fork)
	if err != nil {
		return "", err
	}
	changes := []engines.SyncChange{}
	nodes := map[string]dal.SyncNode{}
	for _, node := range bm.Nodes {
		nodes[node.Path] = node
	}
	for _, node := range lm.Nodes {
		old, exists := nodes[node.Path]
		if !exists {
			changes = append(changes, engines.SyncChange{Path: node.Path, Kind: "added"})
		} else if old != node {
			changes = append(changes, engines.SyncChange{Path: node.Path, Kind: "modified"})
		}
		delete(nodes, node.Path)
	}
	for path := range nodes {
		changes = append(changes, engines.SyncChange{Path: path, Kind: "deleted"})
	}
	slices.SortFunc(changes, func(a, b engines.SyncChange) int { return strings.Compare(a.Path, b.Path) })
	var out strings.Builder
	for _, change := range changes {
		fmt.Fprintf(&out, "%s %s\n", change.Kind, change.Path)
		before, _ := os.ReadFile(filepath.Join(base, change.Path))
		after, _ := os.ReadFile(filepath.Join(fork, change.Path))
		if !utf8.Valid(before) || !utf8.Valid(after) || strings.ContainsRune(string(before), 0) || strings.ContainsRune(string(after), 0) {
			out.WriteString("  binary content changed\n")
			continue
		}
		if len(before) > 0 {
			fmt.Fprintf(&out, "--- baseline/%s\n-%s\n", change.Path, strings.ReplaceAll(string(before), "\n", "\n-"))
		}
		if len(after) > 0 {
			fmt.Fprintf(&out, "+++ local/%s\n+%s\n", change.Path, strings.ReplaceAll(string(after), "\n", "\n+"))
		}
	}
	return out.String(), nil
}
