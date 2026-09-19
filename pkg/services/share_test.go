package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestCreateShareContentIncludesSelectedEntryTreeAndEmitsApplyCommand(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/references/example.md", "ref body")
	writeFile(t, root, "skills/local/demo/scripts/run.sh", "#!/bin/sh\n")
	writeFile(t, root, "skills/local/sibling/SKILL.md", frontmatter("sibling", "sibling"))

	result, err := svc.CreateShare(t.Context(), []string{"demo"}, false)
	require.NoError(t, err)
	require.Equal(t, ShareModeContent, result.Mode)
	require.Contains(t, result.Command, "skm share apply 'skm1z")

	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	require.Len(t, decoded.Content, 1)
	entry := decoded.Content[0]
	require.Equal(t, "demo", entry.Name)

	paths := make([]string, 0, len(entry.Files))
	for _, f := range entry.Files {
		paths = append(paths, f.Path)
	}
	require.ElementsMatch(t, []string{"SKILL.md", "references/example.md", "scripts/run.sh"}, paths)
}

func TestCreateShareContentExcludesGitAndRepositoryLevelFiles(t *testing.T) {
	svc, _, root := exportFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills/local/demo/.git"), 0o755))
	writeFile(t, root, "skills/local/demo/.git/HEAD", "ref: refs/heads/main")
	writeFile(t, root, "README.md", "repo readme")

	result, err := svc.CreateShare(t.Context(), []string{"demo"}, false)
	require.NoError(t, err)
	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	for _, f := range decoded.Content[0].Files {
		require.NotContains(t, f.Path, ".git")
		require.NotEqual(t, "README.md", f.Path)
	}
}

func TestCreateShareRejectsUnknownExplicitName(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.CreateShare(t.Context(), []string{"missing"}, false)
	require.Error(t, err)
}

func TestCreateShareUpstreamIncludesOnlySourceIdentity(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)

	result, err := svc.CreateShare(t.Context(), []string{"demo"}, true)
	require.NoError(t, err)
	require.Equal(t, ShareModeUpstream, result.Mode)

	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	require.Equal(t, []UpstreamEntry{{Name: "demo", Kind: common.KindSkill, Source: "https://github.com/acme/skills"}}, decoded.Upstream)
}

func TestCreateShareUpstreamRejectsLocalOnlyExplicitEntry(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.CreateShare(t.Context(), []string{"demo"}, true)
	require.Error(t, err)
}

func TestCreateShareUpstreamDefaultSelectionWarnsAndSkipsLocalOnly(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)
	writeFile(t, root, "skills/local/local-only/SKILL.md", frontmatter("local-only", "local only"))

	result, err := svc.CreateShare(t.Context(), nil, true)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "demo", result.Entries[0].Name)
	require.NotEmpty(t, result.Warnings)
}

func TestApplyShareContentImportsAndInstallsToCompatibleTargets(t *testing.T) {
	svc, target, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/references/example.md", "ref body")
	create, err := svc.CreateShare(t.Context(), []string{"demo"}, false)
	require.NoError(t, err)

	empty := t.TempDir()
	dest := newCfg(empty, []common.InstallTarget{*target})
	writeFile(t, empty, ".keep", "")
	require.NoError(t, os.MkdirAll(filepath.Join(empty, "skills"), 0o755))
	destSvc, err := New(dest, common.NewLogger(false))
	require.NoError(t, err)

	result, err := destSvc.ApplyShare(context.Background(), create.Payload)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Items, 1)
	require.Equal(t, "installed", result.Items[0].Status)

	imported := destSvc.FindEntry("demo")
	require.NotNil(t, imported)
	require.FileExists(t, filepath.Join(imported.Path, "references", "example.md"))
}

func TestApplyShareUpstreamFetchesThroughImport(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(frontmatter("remote", "remote")), 0o644))

	payload, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "remote", Kind: common.KindSkill, Source: source},
	}})
	require.NoError(t, err)

	dest := t.TempDir()
	svc, err := New(newCfg(dest, nil), common.NewLogger(false))
	require.NoError(t, err)
	result, err := svc.ApplyShare(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "imported", result.Items[0].Status)
	require.NotNil(t, svc.FindEntry("remote"))
}

func TestApplyShareSkipsSameNameCollisionWithoutOverwrite(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/original.txt", "keep me")
	create, err := svc.CreateShare(t.Context(), []string{"demo"}, false)
	require.NoError(t, err)

	result, err := svc.ApplyShare(context.Background(), create.Payload)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "skipped", result.Items[0].Status)
	require.FileExists(t, filepath.Join(root, "skills/local/demo/original.txt"))
}

func TestApplyShareReportsNoCompatibleTargetAndPreservesImport(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(frontmatter("orphan", "orphan")), 0o644))

	payload, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "orphan", Kind: common.KindSkill, Source: source},
	}})
	require.NoError(t, err)

	dest := t.TempDir()
	svc, err := New(newCfg(dest, nil), common.NewLogger(false))
	require.NoError(t, err)
	result, err := svc.ApplyShare(context.Background(), payload)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "imported", result.Items[0].Status)
	require.Equal(t, "no compatible installer target was found", result.Items[0].Reason)
}

func TestApplyShareContinuesIndependentItemsAfterOneFailure(t *testing.T) {
	payload, err := encodeSharePayload(SharePayload{Mode: ShareModeUpstream, Upstream: []UpstreamEntry{
		{Name: "one", Kind: common.KindSkill, Source: "https://github.com/acme/missing-one"},
		{Name: "two", Kind: common.KindCommand, Source: "https://github.com/acme/missing-two"},
	}})
	require.NoError(t, err)
	svc, err := New(newCfg(t.TempDir(), nil), common.NewLogger(false))
	require.NoError(t, err)
	result, err := svc.ApplyShare(context.Background(), payload)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Len(t, result.Items, 2)
	require.Equal(t, "failed", result.Items[0].Status)
	require.Equal(t, "failed", result.Items[1].Status)
}

func TestApplyShareRejectsMalformedPayloadBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	svc, err := New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	before := svc.Scan()

	_, err = svc.ApplyShare(context.Background(), "skm1zNOTVALID000")
	require.Error(t, err)
	require.Equal(t, before, svc.Scan())
}

// Proves SC-002's 1-second local budget.
func TestCreateShareContentCompletesWithinOneSecondFor20Entries(t *testing.T) {
	svc, _, root := exportFixture(t)
	names := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		writeFile(t, root, filepath.Join("skills/local", name, "SKILL.md"), frontmatter(name, name))
		writeFile(t, root, filepath.Join("skills/local", name, "references/example.md"), "reference body")
		names = append(names, name)
	}

	start := time.Now()
	result, err := svc.CreateShare(t.Context(), names, false)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, result.Entries, 20)
	require.Lessf(t, elapsed, time.Second, "share create for 20 entries took %s, want <1s (SC-002)", elapsed)
}
