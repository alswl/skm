package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitCommandCreatesFirstRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "first-skills")
	out, err := runCmd(t, "init", root, "--json")
	require.NoError(t, err)
	require.JSONEq(t, `{"root":"`+root+`","next":"skm import PATH --root `+root+`"}`, out)
	require.DirExists(t, filepath.Join(root, "skills"))
}
