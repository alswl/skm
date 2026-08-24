package builtins

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentsDefinitionDefaultsAndOverride(t *testing.T) {
	defaultTarget := agents().Materialize(Context{Home: "/home/user", Getenv: func(string) string { return "" }})
	require.Equal(t, "/home/user/.agents/skills", defaultTarget.Path)
	overrideTarget := agents().Materialize(Context{Home: "/home/user", Getenv: func(string) string { return "/tmp/agents" }})
	require.Equal(t, "/tmp/agents/skills", overrideTarget.Path)
	require.Equal(t, "kebab-case", overrideTarget.NameRule)
}
