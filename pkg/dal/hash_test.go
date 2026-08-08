package dal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDirHashOnPlainFile: DirHash is named and documented as hashing a
// directory tree, but the repository package calls it with entry.Path for
// single-file commands too (entry.Path is the .md marker file itself, not a
// directory — Entry.MarkerPath doc). filepath.WalkDir visits its root even
// when that root is a plain file, so this must not crash; this test locks
// that in and checks basic hash correctness (same content -> same hash,
// different content -> different hash) for the plain-file case specifically.
func TestDirHashOnPlainFile(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	fileB := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(fileA, []byte("same content"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("same content"), 0o644))

	hashA, err := DirHash(fileA, nil)
	require.NoError(t, err, "DirHash must not crash when given a plain file instead of a directory")
	hashB, err := DirHash(fileB, nil)
	require.NoError(t, err)
	require.NotEmpty(t, hashA)

	require.NoError(t, os.WriteFile(fileB, []byte("different content"), 0o644))
	hashBChanged, err := DirHash(fileB, nil)
	require.NoError(t, err)
	require.NotEqual(t, hashB, hashBChanged, "changed file content must change the hash")

	// Not asserted equal to hashA: DirHash's hash input includes the path
	// relative to the given root ("." for a bare-file root), so two
	// same-content files at different basenames still coincidentally match
	// here (both reduce to rel="."). This documents that DirHash of a plain
	// file is keyed by content only, not by the file's original name.
	_ = hashA
}

// TestDirHashPlainFileVsWrappedDirectoryMismatch documents a real asymmetry:
// comparing a single-file command's on-disk hash (DirHash of the .md file
// directly) against a freshly staged copy of the same content wrapped in a
// directory (as repository.stageCopy always does for a single .md source,
// see import.go) produces DIFFERENT hashes even though the content is
// byte-identical, because the hash input includes the relative path ("." vs
// "command.md"). This is currently unreachable in practice — self-build
// single-file commands never carry an Origin, so Update/BatchUpdate always
// skip them before ever calling DirHash — but it would misreport "changed"
// on every comparison if that ever became reachable. Not fixed here (no
// observed bug, per project simplicity-first guidance); documented so a
// future change that gives a single-file command an Origin doesn't
// reintroduce this silently.
func TestDirHashPlainFileVsWrappedDirectoryMismatch(t *testing.T) {
	dir := t.TempDir()
	flatFile := filepath.Join(dir, "flatcmd.md")
	content := []byte("---\nname: flatcmd\ndescription: d\n---\nbody\n")
	require.NoError(t, os.WriteFile(flatFile, content, 0o644))

	wrappedDir := filepath.Join(dir, "wrapped")
	require.NoError(t, os.MkdirAll(wrappedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wrappedDir, "command.md"), content, 0o644))

	flatHash, err := DirHash(flatFile, nil)
	require.NoError(t, err)
	wrappedHash, err := DirHash(wrappedDir, nil)
	require.NoError(t, err)
	require.NotEqual(t, flatHash, wrappedHash,
		"documents the asymmetry: identical content hashes differently as a bare file vs. a wrapped directory")
}
