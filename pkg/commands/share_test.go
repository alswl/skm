package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testSharePayload = "skm1z2do68zqRpz7n882C14TRQyuJ9yXXmtWcKTZXkCqe5LXomUs7KqFkZTKxstqDRi9yEP86GYMEVPYFQxodz"

func TestShareInstallPreviewsAndRequiresYesForJSON(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	out, err := runCmd(t, "share", "install", testSharePayload, "--root", root, "--config", cfgDir)
	require.NoError(t, err)
	require.Contains(t, out, "install all items?")

	_, err = runCmd(t, "share", "install", testSharePayload, "--root", root, "--config", cfgDir, "--json")
	require.Error(t, err)
}

func TestShareInstallRejectsFileAndStdinForms(t *testing.T) {
	root, cfgDir := cmdFixture(t)
	for _, payload := range []string{"/tmp/share.json", "-"} {
		_, err := runCmd(t, "share", "install", payload, "--root", root, "--config", cfgDir)
		require.Error(t, err)
	}
}
