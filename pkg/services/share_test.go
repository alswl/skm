package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestCreateShareIncludesOnlySourceIdentityForUpstreamOrigin(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)

	result, err := svc.CreateShare(t.Context(), []string{"demo"}, nil)
	require.NoError(t, err)
	require.Contains(t, result.Command, "skm share apply 'skm1z")

	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	require.Equal(t, []ShareEntry{{Name: "demo", Kind: common.KindSkill, Source: "https://github.com/acme/skills"}}, decoded.Entries)
}

// repoAddressFixture builds a git repo with a github.com remote and one
// origin-less (self-build) skill, so CreateShare falls back to a repository
// browse address for it.
func repoAddressFixture(t *testing.T) (svc *Services, root, branch string) {
	t.Helper()
	root = t.TempDir()
	writeSvcFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "demo"))
	require.NoError(t, exec.Command("git", "-C", root, "init", "-q").Run())
	require.NoError(t, exec.Command("git", "-C", root, "add", "-A").Run())
	require.NoError(t, exec.Command("git", "-C", root, "commit", "-q", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", root, "remote", "add", "origin", "git@github.com:acme/skills.git").Run())
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	require.NoError(t, err)
	branch = strings.TrimSpace(string(out))

	svc, err = New(newCfg(root, nil), common.NewLogger(false))
	require.NoError(t, err)
	return svc, root, branch
}

func TestCreateShareFallsBackToRepositoryAddressForLocalOnlyEntry(t *testing.T) {
	svc, _, branch := repoAddressFixture(t)

	result, err := svc.CreateShare(t.Context(), []string{"demo"}, nil)
	require.NoError(t, err)
	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/acme/skills/tree/"+branch+"/skills/local/demo", decoded.Entries[0].Source)
}

func TestCreateShareRejectsLocalOnlyExplicitEntryWithoutRepositoryRemote(t *testing.T) {
	svc, _, _ := exportFixture(t) // origin is git@example.com:… — not a recognized browse host
	_, err := svc.CreateShare(t.Context(), []string{"demo"}, nil)
	require.Error(t, err)
}

func TestCreateShareDefaultSelectionWarnsAndSkipsUnshareableEntry(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)
	writeFile(t, root, "skills/local/local-only/SKILL.md", frontmatter("local-only", "local only"))

	result, err := svc.CreateShare(t.Context(), nil, nil)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "demo", result.Entries[0].Name)
	require.NotEmpty(t, result.Warnings)
}

func TestCreateShareRejectsUnknownExplicitName(t *testing.T) {
	svc, _, _ := exportFixture(t)
	_, err := svc.CreateShare(t.Context(), []string{"missing"}, nil)
	require.Error(t, err)
}

func TestApplyShareFetchesThroughImport(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(frontmatter("remote", "remote")), 0o644))

	payload, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
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

func TestApplyShareInstallsToCompatibleTargets(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(frontmatter("demo", "demo")), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	target := skillTarget("t", targetDir)

	payload, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "demo", Kind: common.KindSkill, Source: source},
	}})
	require.NoError(t, err)

	dest := t.TempDir()
	svc, err := New(newCfg(dest, []common.InstallTarget{target}), common.NewLogger(false))
	require.NoError(t, err)
	result, err := svc.ApplyShare(context.Background(), payload)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "installed", result.Items[0].Status)
}

func TestApplyShareSkipsSameNameCollisionWithoutOverwrite(t *testing.T) {
	svc, _, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/original.txt", "keep me")

	source := filepath.Join(t.TempDir(), "source-skill")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(frontmatter("demo", "demo")), 0o644))
	payload, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
		{Name: "demo", Kind: common.KindSkill, Source: source},
	}})
	require.NoError(t, err)

	result, err := svc.ApplyShare(context.Background(), payload)
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

	payload, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
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
	payload, err := encodeSharePayload(SharePayload{Entries: []ShareEntry{
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

func TestCreateShareCompletesWithinOneSecondFor20Entries(t *testing.T) {
	svc, _, root := exportFixture(t)
	names := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		writeFile(t, root, filepath.Join("skills/local", name, "SKILL.md"), frontmatter(name, name))
		writeFile(t, root, filepath.Join("skills/local", name, "meta.json"),
			`{"address":"https://github.com/acme/`+name+`","mode_id":"local"}`)
		names = append(names, name)
	}

	start := time.Now()
	result, err := svc.CreateShare(t.Context(), names, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, result.Entries, 20)
	require.Lessf(t, elapsed, time.Second, "share create for 20 entries took %s, want <1s", elapsed)
}
