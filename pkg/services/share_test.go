package services

import (
	"context"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestCreateShareSelectsInstalledEntriesAndEmitsCommand(t *testing.T) {
	svc, target, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)
	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	tx := &dal.FileTransaction{}
	_, err := svc.Installer.Install(tx, entry, *target, false)
	require.NoError(t, err)
	tx.Commit()

	result, err := svc.CreateShare(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, []ShareItem{{Source: "https://github.com/acme/skills", Name: "demo", Kind: common.KindSkill}}, result.Items)
	require.Contains(t, result.Command, "skm share install 'skm1z")
	decoded, err := decodeSharePayload(result.Payload)
	require.NoError(t, err)
	require.Equal(t, result.Items, decoded)
}

func TestCreateShareSkipsEntriesWithoutDownloadableSources(t *testing.T) {
	svc, target, root := exportFixture(t)
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"https://github.com/acme/skills","mode_id":"local"}`)
	writeFile(t, root, "skills/local/local-only/SKILL.md", "---\nname: local-only\ndescription: local only\n---\nbody\n")
	entry := svc.FindEntry("demo")
	require.NotNil(t, entry)
	tx := &dal.FileTransaction{}
	_, err := svc.Installer.Install(tx, entry, *target, false)
	require.NoError(t, err)
	tx.Commit()
	localOnly := svc.FindEntry("local-only")
	require.NotNil(t, localOnly)
	tx = &dal.FileTransaction{}
	_, err = svc.Installer.Install(tx, localOnly, *target, false)
	require.NoError(t, err)
	tx.Commit()

	result, err := svc.CreateShare(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "demo", result.Items[0].Name)
	require.Equal(t, []string{"skipped \"local-only\": no downloadable source URL"}, result.Warnings)
}

func TestInstallShareContinuesAfterIndependentFailures(t *testing.T) {
	payload, err := encodeSharePayload([]ShareItem{
		{Source: "https://github.com/acme/missing-one", Name: "one", Kind: common.KindSkill},
		{Source: "https://github.com/acme/missing-two", Name: "two", Kind: common.KindCommand},
	})
	require.NoError(t, err)
	svc, err := New(newCfg(t.TempDir(), nil), common.NewLogger(false))
	require.NoError(t, err)
	result, err := svc.InstallShare(context.Background(), payload)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Len(t, result.Items, 2)
	require.Equal(t, "failed", result.Items[0].Status)
	require.Equal(t, "failed", result.Items[1].Status)
}
