package engines

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alswl/skm/skm/pkg/dal"
)

type PatchResult struct {
	Failures []SyncChange
	Patch    []byte
}

// ApplyCustomization only writes candidate. The original three trees remain
// available even when a text tool leaves a partially applied result.
func ApplyCustomization(ctx context.Context, base, local, upstream, candidate string) (PatchResult, error) {
	result := PatchResult{Failures: []SyncChange{}}
	roots := []string{base, local, upstream}
	trees := make([]map[string]dal.SyncNode, 3)
	all := map[string]bool{}
	for i, root := range roots {
		m, err := dal.SyncTree(root)
		if err != nil {
			return result, err
		}
		trees[i] = map[string]dal.SyncNode{}
		for _, n := range m.Nodes {
			trees[i][n.Path] = n
			all[n.Path] = true
		}
	}
	paths := make([]string, 0, len(all))
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if err := os.MkdirAll(candidate, 0700); err != nil {
		return result, err
	}
	var conflictedSubtrees []string
	for _, path := range paths {
		skip := false
		for _, parent := range conflictedSubtrees {
			if strings.HasPrefix(path, parent+"/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		b, l, u := trees[0][path], trees[1][path], trees[2][path]
		selected := u
		source := upstream
		var merged []byte
		hasMerged := false
		failed := false
		markerLabel := ""
		// A directory's node identity does not capture descendant edits. Resolve
		// structural conflicts at the subtree root before materializing children.
		structural := (b.Kind == "directory" || l.Kind == "directory" || u.Kind == "directory") && l.Kind != u.Kind &&
			!syncSubtreeEqual(trees[0], trees[1], path) && !syncSubtreeEqual(trees[0], trees[2], path) && !syncSubtreeEqual(trees[1], trees[2], path)
		switch {
		case structural:
			result.Failures = append(result.Failures, SyncChange{Path: path, Kind: "structure"})
			conflictedSubtrees = append(conflictedSubtrees, path)
			selected = l
			source = local
			if l.Kind == "directory" {
				dest, err := dal.SyncPath(candidate, filepath.FromSlash(path))
				if err != nil {
					return result, err
				}
				if err = dal.CopySyncTree(filepath.Join(local, filepath.FromSlash(path)), dest); err != nil {
					return result, err
				}
				continue
			}
		case l == b:
		case u == b:
			selected = l
			source = local
		case l == u:
		case b.Kind == "file" && l.Kind == "file" && u.Kind == "file":
			bd, err := os.ReadFile(filepath.Join(base, path))
			if err != nil {
				return result, err
			}
			ld, err := os.ReadFile(filepath.Join(local, path))
			if err != nil {
				return result, err
			}
			ud, err := os.ReadFile(filepath.Join(upstream, path))
			if err != nil {
				return result, err
			}
			if bytes.Equal(ld, bd) {
				merged = ud
				hasMerged = true
			} else if bytes.Equal(ud, bd) || bytes.Equal(ld, ud) {
				merged = ld
				hasMerged = true
			} else if isSyncText(bd) && isSyncText(ld) && isSyncText(ud) {
				data, patch, clean, err := applyTextPatch(ctx, bd, ld, ud)
				if err != nil {
					return result, err
				}
				result.Patch = append(result.Patch, []byte("# "+path+"\n")...)
				result.Patch = append(result.Patch, patch...)
				merged = data
				hasMerged = true
				failed = !clean
				if !clean && bytes.Contains(merged, []byte("<<<<<<< ours\n")) {
					markerLabel = "skm-" + rand.Text()
					merged = bytes.ReplaceAll(merged, []byte("<<<<<<< ours\n"), []byte("<<<<<<< "+markerLabel+"-local\n"))
					merged = bytes.ReplaceAll(merged, []byte(">>>>>>> theirs"), []byte(">>>>>>> "+markerLabel+"-upstream"))
				}

			} else {
				failed = true
			}
			if l.Executable == b.Executable {
				selected.Executable = u.Executable
			} else {
				selected.Executable = l.Executable
			}
		default:
			failed = true
		}
		if failed {
			result.Failures = append(result.Failures, SyncChange{Path: path, Kind: "patch_failed", MarkerLabel: markerLabel})
			selected = l
			source = local
			if !hasMerged {
				merged = nil
			}
		}
		if selected.Kind == "" {
			continue
		}
		dest, err := dal.SyncPath(candidate, filepath.FromSlash(path))
		if err != nil {
			return result, err
		}
		if selected.Kind == "directory" {
			err = os.MkdirAll(dest, 0755)
		} else {
			err = os.MkdirAll(filepath.Dir(dest), 0755)
			if err == nil {
				data := merged
				if !hasMerged {
					data, err = os.ReadFile(filepath.Join(source, path))
				}
				if err == nil {
					mode := os.FileMode(0644)
					if selected.Executable {
						mode = 0755
					}
					err = os.WriteFile(dest, data, mode)
				}
			}
		}
		if err != nil {
			result.Failures = append(result.Failures, SyncChange{Path: path, Kind: "structure"})
		}
	}
	return result, nil
}
func syncSubtreeEqual(a, b map[string]dal.SyncNode, path string) bool {
	if a[path] != b[path] {
		return false
	}
	prefix := path + "/"
	for p, n := range a {
		if strings.HasPrefix(p, prefix) && b[p] != n {
			return false
		}
	}
	for p, n := range b {
		if strings.HasPrefix(p, prefix) && a[p] != n {
			return false
		}
	}
	return true
}

func isSyncText(b []byte) bool { return utf8.Valid(b) && !bytes.ContainsRune(b, 0) }

func applyTextPatch(ctx context.Context, b, l, u []byte) ([]byte, []byte, bool, error) {
	dir, err := os.MkdirTemp("", "skm-patch-*")
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// Separate HOME/config and fixed filenames keep source .gitattributes,
	// hooks, and user Git environment out of the merge operation.
	home := filepath.Join(dir, "home")
	if err = os.Mkdir(home, 0700); err != nil {
		return nil, nil, false, err
	}
	run := func(input []byte, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.autocrlf=false", "-c", "core.hooksPath=/dev/null", "-c", "merge.conflictStyle=diff3"}, args...)...)
		cmd.Dir = dir
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "GIT_") && !strings.HasPrefix(v, "HOME=") && !strings.HasPrefix(v, "XDG_CONFIG_HOME=") {
				cmd.Env = append(cmd.Env, v)
			}
		}
		cmd.Env = append(cmd.Env, "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LC_ALL=C")
		cmd.Stdin = bytes.NewReader(input)
		return cmd.CombinedOutput()
	}
	check := func(args ...string) ([]byte, error) {
		out, err := run(nil, args...)
		if err != nil {
			return nil, fmt.Errorf("patch tool: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return out, nil
	}
	if _, err = check("init", "-q"); err != nil {
		return nil, nil, false, err
	}
	content := filepath.Join(dir, "content")
	stage := func(data []byte) error {
		if err := os.WriteFile(content, data, 0600); err != nil {
			return err
		}
		_, err := check("add", "--", "content")
		return err
	}
	if err = stage(b); err != nil {
		return nil, nil, false, err
	}
	tree, err := check("write-tree")
	if err != nil {
		return nil, nil, false, err
	}
	if err = stage(l); err != nil {
		return nil, nil, false, err
	}
	patch, err := check("diff", "--cached", "--binary", "--full-index", strings.TrimSpace(string(tree)), "--", "content")
	if err != nil {
		return nil, nil, false, err
	}
	if err = stage(u); err != nil {
		return nil, nil, false, err
	}
	out, applyErr := run(patch, "apply", "--3way", "--whitespace=nowarn", "-")
	if applyErr != nil {
		if ctx.Err() != nil {
			return nil, patch, false, ctx.Err()
		}
		exit, ok := applyErr.(*exec.ExitError)
		if !ok || exit.ExitCode() != 1 {
			return nil, patch, false, fmt.Errorf("patch tool: %s: %w", strings.TrimSpace(string(out)), applyErr)
		}
	}
	data, err := os.ReadFile(content)
	return data, patch, applyErr == nil, err
}

// GenerateCustomizationPatch captures the whole B→L delta, including simple
// changes that need no three-way assistance. Empty directories remain in the
// accompanying manifests because Git patches do not represent them.
func GenerateCustomizationPatch(ctx context.Context, base, local string) ([]byte, error) {
	bm, err := dal.SyncTree(base)
	if err != nil {
		return nil, err
	}
	lm, err := dal.SyncTree(local)
	if err != nil {
		return nil, err
	}
	if bm.Digest == lm.Digest {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "skm-diff-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, item := range []struct{ src, name string }{{base, "base"}, {local, "local"}} {
		if err = dal.CopySyncTree(item.src, filepath.Join(dir, item.name)); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, "git", "-c", "core.autocrlf=false", "-c", "core.hooksPath=/dev/null", "diff", "--no-index", "--binary", "--no-ext-diff", "--no-textconv", "--", "base", "local")
	cmd.Dir = dir
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") && !strings.HasPrefix(v, "HOME=") && !strings.HasPrefix(v, "XDG_CONFIG_HOME=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+dir, "XDG_CONFIG_HOME="+dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LC_ALL=C")
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	patch, err := cmd.Output()
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("patch tool: %s: %w", diagnostic.String(), err)
		}
	}
	return patch, nil
}
