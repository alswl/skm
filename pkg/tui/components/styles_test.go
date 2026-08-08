package components

import (
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestInstallIconDistinguishesEachState(t *testing.T) {
	cases := map[common.InstallState]string{
		common.InstallInstalled: "✓",
		common.InstallConflict:  "✗",
		common.InstallDangling:  "⚠",
		common.InstallAbsent:    "",
		InstallNA:               "",
	}
	for state, want := range cases {
		got, _ := InstallIcon(state)
		require.Equal(t, want, got, state)
	}
}
