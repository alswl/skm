package engines

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/stretchr/testify/require"
)

func TestTextPatchProbe(t *testing.T) {
	base := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n"
	tests := []struct {
		name, local, upstream, want string
		clean                       bool
	}{
		{"independent", "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n", "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n", true},
		{"absorbed", "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n", "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n", "ONE\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nNINE\n", true},
		{"conflicting", "LOCAL\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n", "UPSTREAM\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, patch, clean, err := applyTextPatch(context.Background(), []byte(base), []byte(tt.local), []byte(tt.upstream))
			require.NoError(t, err)
			require.NotEmpty(t, patch)
			require.Equal(t, tt.clean, clean)
			if clean {
				require.Equal(t, tt.want, string(got))
			}
		})
	}
}
func TestPatchTreeDeleteModify(t *testing.T) {
	b, l, u, c := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(b, "x"), []byte("base"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(l, "x"), []byte("local"), 0600))
	result, err := ApplyCustomization(context.Background(), b, l, u, c)
	require.NoError(t, err)
	require.Len(t, result.Failures, 1)
	got, err := os.ReadFile(filepath.Join(c, "x"))
	require.NoError(t, err)
	require.Equal(t, "local", string(got))
}

func TestCustomizationPatchIncludesSingleSidedAndBinaryChanges(t *testing.T) {
	b, l := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(b, "old.txt"), []byte("old\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(l, "new.txt"), []byte("new\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(l, "asset.bin"), []byte{0, 1, 2}, 0600))
	patch, err := GenerateCustomizationPatch(context.Background(), b, l)
	require.NoError(t, err)
	require.Contains(t, string(patch), "old.txt")
	require.Contains(t, string(patch), "new.txt")
	require.Contains(t, string(patch), "GIT binary patch")
}

func TestPatchTreeMatrix(t *testing.T) {
	type files map[string][]byte
	tests := []struct {
		name          string
		b, l, u, want files
		fail          bool
	}{
		{"clean", files{"x": []byte("old")}, files{"x": []byte("old")}, files{"x": []byte("new")}, files{"x": []byte("new")}, false},
		{"local only", files{"x": []byte("old")}, files{"x": []byte("local")}, files{"x": []byte("old")}, files{"x": []byte("local")}, false},
		{"same", files{"x": []byte("old")}, files{"x": []byte("same")}, files{"x": []byte("same")}, files{"x": []byte("same")}, false},
		{"local add", files{}, files{"x": []byte("local")}, files{"y": []byte("upstream")}, files{"x": []byte("local"), "y": []byte("upstream")}, false},
		{"local delete", files{"x": []byte("old")}, files{}, files{"x": []byte("old")}, files{}, false},
		{"upstream delete", files{"x": []byte("old")}, files{"x": []byte("old")}, files{}, files{}, false},
		{"different add", files{}, files{"x": []byte("local")}, files{"x": []byte("upstream")}, files{"x": []byte("local")}, true},
		{"binary local only", files{"x": {0, 1}}, files{"x": {0, 2}}, files{"x": {0, 1}}, files{"x": {0, 2}}, false},
		{"binary differs", files{"x": {0, 1}}, files{"x": {0, 2}}, files{"x": {0, 3}}, files{"x": {0, 2}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := []string{t.TempDir(), t.TempDir(), t.TempDir()}
			for i, tree := range []files{tt.b, tt.l, tt.u} {
				for name, data := range tree {
					require.NoError(t, os.WriteFile(filepath.Join(roots[i], name), data, 0600))
				}
			}
			candidate := t.TempDir()
			result, err := ApplyCustomization(context.Background(), roots[0], roots[1], roots[2], candidate)
			require.NoError(t, err)
			require.Equal(t, tt.fail, len(result.Failures) > 0)
			names, err := os.ReadDir(candidate)
			require.NoError(t, err)
			require.Len(t, names, len(tt.want))
			for path, data := range tt.want {
				got, err := os.ReadFile(filepath.Join(candidate, path))
				require.NoError(t, err)
				require.Equal(t, data, got)
			}
		})
	}
}

func TestPatchTreeStructureConflictPreservesLocalSubtree(t *testing.T) {
	for _, side := range []string{"upstream replaces directory", "local replaces directory"} {
		t.Run(side, func(t *testing.T) {
			base, local, upstream, candidate := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			for _, root := range []string{base, local, upstream} {
				require.NoError(t, os.Mkdir(filepath.Join(root, "d"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "d", "a"), []byte("base\n"), 0644))
			}
			replacement, edited := upstream, local
			if side == "local replaces directory" {
				replacement, edited = local, upstream
			}
			require.NoError(t, os.RemoveAll(filepath.Join(replacement, "d")))
			require.NoError(t, os.WriteFile(filepath.Join(replacement, "d"), []byte("replacement\n"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(edited, "d", "b"), []byte("new descendant\n"), 0644))
			result, err := ApplyCustomization(context.Background(), base, local, upstream, candidate)
			require.NoError(t, err)
			require.Contains(t, result.Failures, SyncChange{Path: "d", Kind: "structure"})
			got, err := dal.SyncTree(candidate)
			require.NoError(t, err)
			want, err := dal.SyncTree(local)
			require.NoError(t, err)
			require.Equal(t, want.Digest, got.Digest, "failed candidate must retain the complete local subtree")
		})
	}
}
