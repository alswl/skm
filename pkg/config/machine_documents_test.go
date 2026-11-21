package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestMachineDocumentsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machine.yaml")
	doc, err := LoadMachineDocument(path)
	require.NoError(t, err)
	require.Equal(t, 1, doc.Version)
	require.NoFileExists(t, path)

	valid := "version: 1\ndirectories:\n  demo:\n    tags: [go]\ninstalls:\n  - skill: skills/local/review\n    target: codex\n    scope: {kind: directory, name: demo}\n"
	require.NoError(t, os.WriteFile(path, []byte(valid), 0o600))
	doc, err = LoadMachineDocument(path)
	require.NoError(t, err)
	require.Len(t, doc.Installs, 1)

	for _, invalid := range []string{
		"", "# empty document\n", "null\n",
		valid + "unknown: true\n",
		"version: 1\nversion: 1\n",
		"version: 1\ndirectories:\n  demo: {tags: []}\n  demo: {tags: []}\n",
		"version: 1\ndirectories:\n  demo: {tags: [], path: /tmp/project}\n",
		"version: 1\npaths: {demo: /tmp/project}\n",
		"version: 1\ninstalls:\n  - skill: skills/a\n    target: codex\n    scope: {kind: user, name: bad}\n",
		"version: 1\ninstalls:\n  - skill: skills/a\n    target: unknown\n    scope: {kind: tag, name: team}\n",
		"version: 2\n",
		"directories: {}\n",
		"version: 1\ninstalls:\n  - skill: ../outside\n    target: codex\n    scope: {kind: user}\n",
		"version: 1\ninstalls:\n  - skill: skills/a\n    target: codex\n    scope: {kind: directory, name: missing}\n",
	} {
		require.NoError(t, os.WriteFile(path, []byte(invalid), 0o600))
		_, err := LoadMachineDocument(path)
		require.Error(t, err, invalid)
	}

	pathsFile := filepath.Join(dir, "paths.yaml")
	require.NoError(t, os.WriteFile(pathsFile, []byte("version: 1\npaths:\n  demo: relative\n"), 0o600))
	_, err = LoadMachinePaths(pathsFile, doc)
	require.Error(t, err)
}

func TestMachinePathsCanonicalizeAndRejectDuplicates(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(root, alias))
	path := filepath.Join(t.TempDir(), "paths.yaml")
	doc := MachineDocument{Version: 1, Directories: map[string]common.MachineDirectory{"one": {}, "two": {}}}
	require.NoError(t, os.WriteFile(path, []byte("version: 1\npaths:\n  one: "+alias+"\n"), 0o600))
	loaded, err := LoadMachinePaths(path, doc)
	require.NoError(t, err)
	require.Equal(t, root, loaded.Paths["one"])
	require.NoError(t, os.WriteFile(path, []byte("version: 1\npaths:\n  one: "+alias+"\n  two: "+root+"\n"), 0o600))
	_, err = LoadMachinePaths(path, doc)
	require.ErrorContains(t, err, "share path")
	missingThroughAlias := filepath.Join(alias, "not-created")
	missingPhysical := filepath.Join(root, "not-created")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\npaths:\n  one: "+missingThroughAlias+"\n  two: "+missingPhysical+"\n"), 0o600))
	_, err = LoadMachinePaths(path, doc)
	require.ErrorContains(t, err, "share path")
	for _, invalid := range []string{"", "version: 1\npaths: {unknown: /tmp/example}\n", "version: 1\npaths: {one: /tmp/example}\ndirectories: {}\n", "version: 1\npaths: {one: /tmp/example, one: /tmp/other}\n"} {
		require.NoError(t, os.WriteFile(path, []byte(invalid), 0o600))
		_, err = LoadMachinePaths(path, doc)
		require.Error(t, err)
	}
}

func TestMachinePathsFileAndAtomicWrite(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(root, alias))
	first, err := MachinePathsFile(configDir, root)
	require.NoError(t, err)
	second, err := MachinePathsFile(configDir, alias)
	require.NoError(t, err)
	require.Equal(t, first, second)

	doc := MachineDocument{Version: 1}
	require.NoError(t, WriteMachineYAML(first, doc, 0o600))
	loaded, err := LoadMachineDocument(first)
	require.NoError(t, err)
	require.Equal(t, doc.Version, loaded.Version)
	info, err := os.Stat(first)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestMachineWriteFailurePreservesDestination(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "machine.yaml")
	require.NoError(t, os.Mkdir(destination, 0o700))
	sentinel := filepath.Join(destination, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))
	require.Error(t, WriteMachineYAML(destination, MachineDocument{Version: 1}, 0o644))
	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	temporary, err := filepath.Glob(filepath.Join(parent, ".machine-*"))
	require.NoError(t, err)
	require.Empty(t, temporary)
}
