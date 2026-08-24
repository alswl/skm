package builtins

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllStableOrder(t *testing.T) {
	defs := All()
	require.Equal(t, []string{"claude-skills", "claude-commands", "codex", "pi", "dsh", "agents"}, []string{
		defs[0].Name, defs[1].Name, defs[2].Name, defs[3].Name, defs[4].Name, defs[5].Name,
	})
}
