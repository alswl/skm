package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/stretchr/testify/require"
)

func findEntryByName(t *testing.T, root, name string) *common.Entry {
	t.Helper()
	for _, e := range NewRepository(root).Scan() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entry %q not found", name)
	return nil
}

func TestUpdateEntryReplacesAndPreservesOrigin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "version one"))
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"/src","mode_id":"local"}`)
	entry := findEntryByName(t, root, "demo")

	staged := t.TempDir()
	writeFile(t, staged, "SKILL.md", frontmatter("demo", "version two"))

	res, err := NewRepository(root).UpdateEntry(context.Background(), entry, staged)
	require.NoError(t, err)
	require.True(t, res.Changed)

	marker := filepath.Join(root, "skills/local/demo/SKILL.md")
	data, _ := os.ReadFile(marker)
	require.Contains(t, string(data), "version two", "content atomically replaced")
	// Origin preserved through the replace (FR-023).
	require.FileExists(t, filepath.Join(root, "skills/local/demo/meta.json"))
}

func TestUpdateEntryCurrentWhenByteIdentical(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "same"))
	writeFile(t, root, "skills/local/demo/meta.json", `{"address":"/src","mode_id":"local"}`)
	entry := findEntryByName(t, root, "demo")

	staged := t.TempDir()
	writeFile(t, staged, "SKILL.md", frontmatter("demo", "same"))

	res, err := NewRepository(root).UpdateEntry(context.Background(), entry, staged)
	require.NoError(t, err)
	require.False(t, res.Changed, "byte-identical content excluding meta.json is current")
}

func TestUpdateEntryFailurePreservesOldContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "skills/local/demo/SKILL.md", frontmatter("demo", "precious"))
	entry := findEntryByName(t, root, "demo")

	// A fetched copy with the wrong name fails validation.
	staged := t.TempDir()
	writeFile(t, staged, "SKILL.md", frontmatter("other-name", "different"))

	_, err := NewRepository(root).UpdateEntry(context.Background(), entry, staged)
	require.Error(t, err)
	data, _ := os.ReadFile(filepath.Join(root, "skills/local/demo/SKILL.md"))
	require.Contains(t, string(data), "precious", "old content preserved on failure")
}
