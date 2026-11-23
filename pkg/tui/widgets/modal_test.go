package widgets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfirmDiffWrapsLongPaths(t *testing.T) {
	path := "/" + strings.Repeat("long-directory/", 8) + "final-skill"
	c := &Confirm{Diff: "install " + path, ShowDiff: true}
	view := ConfirmView(40, 15, "", c)
	require.Contains(t, view, "final-skill")
}
