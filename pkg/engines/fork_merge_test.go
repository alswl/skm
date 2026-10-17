package engines

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type tree map[string]string // relative path -> content; "-> target" is a symlink

func writeTree(t *testing.T, root string, files tree) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		if len(content) > 3 && content[:3] == "-> " {
			require.NoError(t, os.Symlink(content[3:], p))
			continue
		}
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

func readTree(t *testing.T, root string) tree {
	t.Helper()
	out := tree{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.Type()&os.ModeSymlink != 0 {
			link, _ := os.Readlink(path)
			out[rel] = "-> " + link
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		out[rel] = string(data)
		return nil
	}))
	return out
}

const ten = "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"

func TestMergeForkFileMatrix(t *testing.T) {
	base := tree{"SKILL.md": skillMD, "keep.txt": "keep\n", "edit.txt": ten, "gone.txt": "bye\n"}
	with := func(over tree, drop ...string) tree {
		out := tree{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range over {
			out[k] = v
		}
		for _, k := range drop {
			delete(out, k)
		}
		return out
	}
	cases := []struct {
		name     string
		fork     tree
		upstream tree
		want     tree // nil: the merge must fail with wantErr
		wantErr  string
	}{
		{name: "fork deletes a file upstream left alone",
			fork: with(nil, "gone.txt"), upstream: with(tree{"keep.txt": "keep v2\n"}),
			want: with(tree{"keep.txt": "keep v2\n"}, "gone.txt")},
		{name: "upstream deletes a file the fork left alone",
			fork: with(tree{"keep.txt": "mine\n"}), upstream: with(nil, "gone.txt"),
			want: with(tree{"keep.txt": "mine\n"}, "gone.txt")},
		{name: "both delete the same file",
			fork: with(nil, "gone.txt"), upstream: with(nil, "gone.txt"),
			want: with(tree{"keep.txt": "keep\n"}, "gone.txt")},
		{name: "fork deletes what upstream modified",
			fork: with(nil, "gone.txt"), upstream: with(tree{"gone.txt": "changed\n"}),
			wantErr: "conflict in gone.txt"},
		{name: "upstream deletes what the fork modified",
			fork: with(tree{"gone.txt": "mine\n"}), upstream: with(nil, "gone.txt"),
			wantErr: "conflict in gone.txt"},
		{name: "each side adds a different file, fork inside a new directory",
			fork: with(tree{"extra/deep/mine.txt": "m\n"}), upstream: with(tree{"theirs.txt": "t\n"}),
			want: with(tree{"extra/deep/mine.txt": "m\n", "theirs.txt": "t\n"})},
		{name: "both add the same file identically",
			fork: with(tree{"new.txt": "same\n"}), upstream: with(tree{"new.txt": "same\n"}),
			want: with(tree{"new.txt": "same\n"})},
		{name: "both add the same path differently",
			fork: with(tree{"new.txt": "mine\n"}), upstream: with(tree{"new.txt": "theirs\n"}),
			wantErr: "conflict in new.txt"},
		{name: "edits in different regions of one file",
			fork: with(tree{"edit.txt": "1 mine\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"}), upstream: with(tree{"edit.txt": "1\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n"}),
			want: with(tree{"edit.txt": "1 mine\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n"})},
		{name: "same edit on both sides stays single",
			fork: with(tree{"edit.txt": "1 same\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"}), upstream: with(tree{"edit.txt": "1 same\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n"}),
			want: with(tree{"edit.txt": "1 same\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n"})},
		{name: "fork adds a symlink",
			fork: with(tree{"link": "-> keep.txt"}), upstream: with(tree{"keep.txt": "keep v2\n"}),
			want: with(tree{"link": "-> keep.txt", "keep.txt": "keep v2\n"})},
		{name: "symlink retargeted on both sides",
			fork:     with(tree{"link": "-> gone.txt"}),
			upstream: with(tree{"link": "-> edit.txt"}),
			wantErr:  "conflict in link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			up, fork, fetched := filepath.Join(root, "skills/local/review"), filepath.Join(root, "skills-fork/local/review"), t.TempDir()
			baseCopy := base
			if tc.name == "symlink retargeted on both sides" {
				baseCopy = with(tree{"link": "-> keep.txt"})
			}
			writeTree(t, up, baseCopy)
			writeTree(t, fork, tc.fork)
			writeTree(t, fetched, tc.upstream)
			upstreamBefore := readTree(t, up)

			m, err := NewRepository(root).MergeFork(context.Background(), up, fork, fetched, ForkMergeTimeout)
			require.Equal(t, upstreamBefore, readTree(t, up), "merging never touches the repository")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			defer m.Cleanup()
			require.True(t, m.Changed)
			require.Equal(t, tc.want, readTree(t, m.Merged))
			require.Equal(t, readTree(t, fetched), readTree(t, m.NewUpstream), "the new upstream is kept pristine")
		})
	}
}

func TestMergeForkKeepsForkFilePermissions(t *testing.T) {
	root := t.TempDir()
	up, fork, fetched := filepath.Join(root, "skills/local/review"), filepath.Join(root, "skills-fork/local/review"), t.TempDir()
	for _, dir := range []string{up, fetched} {
		writeTree(t, dir, tree{"SKILL.md": skillMD, "run.sh": "#!/bin/sh\n" + ten, "tool.sh": "#!/bin/sh\necho v1\n"})
	}
	writeTree(t, fork, tree{"SKILL.md": skillMD, "run.sh": "#!/bin/sh\n" + ten, "tool.sh": "#!/bin/sh\necho mine\n"})
	require.NoError(t, os.Chmod(filepath.Join(fork, "tool.sh"), 0o755))
	// Upstream edits run.sh far from the top; the fork edits it at the top
	// and marks it executable, so it is merged rather than copied.
	require.NoError(t, os.WriteFile(filepath.Join(fork, "run.sh"), []byte("#!/bin/sh\n1 mine\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(fork, "run.sh"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fetched, "run.sh"), []byte("#!/bin/sh\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fetched, "SKILL.md"), []byte(skillMD+"new\n"), 0o644))

	m, err := NewRepository(root).MergeFork(context.Background(), up, fork, fetched, ForkMergeTimeout)
	require.NoError(t, err)
	defer m.Cleanup()
	for _, name := range []string{"tool.sh", "run.sh"} {
		fi, err := os.Stat(filepath.Join(m.Merged, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), fi.Mode().Perm(), name+" keeps the fork's executable bit")
	}
	merged, err := os.ReadFile(filepath.Join(m.Merged, "run.sh"))
	require.NoError(t, err)
	require.Equal(t, "#!/bin/sh\n1 mine\n2\n3\n4\n5\n6\n7\n8\n9\n10 theirs\n", string(merged))
}

func TestMergeForkNoChangeUpstreamLeavesFork(t *testing.T) {
	root := t.TempDir()
	up, fork, fetched := filepath.Join(root, "skills/local/review"), filepath.Join(root, "skills-fork/local/review"), t.TempDir()
	writeTree(t, up, tree{"SKILL.md": skillMD, "a.txt": ten})
	writeTree(t, fork, tree{"SKILL.md": skillMD, "a.txt": "mine\n"})
	writeTree(t, fetched, tree{"SKILL.md": skillMD, "a.txt": ten})
	m, err := NewRepository(root).MergeFork(context.Background(), up, fork, fetched, ForkMergeTimeout)
	require.NoError(t, err)
	require.False(t, m.Changed)
	require.Empty(t, m.NewUpstream, "nothing is left to clean up or replace")
	require.Empty(t, m.Merged)
}
