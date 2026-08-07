package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeRoot_SelfIsRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "skills")
	if err := os.MkdirAll(filepath.Join(root, "skills", "local", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "commands", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("NormalizeRoot(%q) = %q, want %q", root, got, root)
	}
}

func TestNormalizeRoot_SkillsSubdirPromotesToParent(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeRoot(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("NormalizeRoot(%q) = %q, want %q", skillsDir, got, root)
	}
}

func TestNormalizeRoot_TildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := filepath.Join(tmp, "ws", "skills")
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeRoot("~/ws/skills")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("NormalizeRoot(~/ws/skills) = %q, want %q", got, root)
	}
}
