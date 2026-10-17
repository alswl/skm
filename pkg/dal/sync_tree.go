package dal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SyncNode struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Hash string `json:"hash,omitempty"`
	// Mode is deliberately outside the content manifest digest: sync conflict
	// detection is content based, while transaction backups still preserve it.
	Mode       uint32 `json:"-"`
	Executable bool   `json:"executable,omitempty"`
}
type SyncManifest struct {
	Nodes  []SyncNode `json:"nodes"`
	Digest string     `json:"digest"`
}

// SyncPath rejects symlink ancestors as well as lexical path traversal.
func SyncPath(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) || rel == "." {
		return "", fmt.Errorf("unsafe sync path %q", rel)
	}
	path := root
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		st, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("sync path contains symlink: %s", path)
		}
	}
	return path, nil
}

func SyncTree(root string) (SyncManifest, error) {
	return syncTree(root, false)
}

// PayloadSyncTree describes Skill content without skm's root management file.
// Transaction fingerprints continue to use SyncTree so metadata edits remain
// visible to acceptance and recovery checks.
func PayloadSyncTree(root string) (SyncManifest, error) {
	return syncTree(root, true)
}

func syncTree(root string, excludeMeta bool) (SyncManifest, error) {
	m := SyncManifest{Nodes: []SyncNode{}}
	st, err := os.Lstat(root)
	if err != nil {
		return m, err
	}
	if !st.IsDir() {
		return m, fmt.Errorf("sync root is not a directory: %s", root)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excludeMeta && rel == metaFileName {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("invalid management meta: %s", path)
			}
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		n := SyncNode{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm())}
		switch {
		case d.IsDir():
			n.Kind = "directory"
		case info.Mode().IsRegular():
			n.Kind = "file"
			n.Executable = info.Mode().Perm()&0111 != 0
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			n.Hash = hex.EncodeToString(sum[:])
		default:
			return fmt.Errorf("unsupported_content: %s (%s)", rel, info.Mode())
		}
		m.Nodes = append(m.Nodes, n)
		return nil
	})
	if err != nil {
		return m, err
	}
	data, err := json.Marshal(m.Nodes)
	if err != nil {
		return m, err
	}
	sum := sha256.Sum256(data)
	m.Digest = hex.EncodeToString(sum[:])
	return m, nil
}

func CopySyncTree(src, dst string) error {
	return copySyncTree(src, dst, false)
}

// CopyPayloadSyncTree copies installable Skill content, omitting only the
// skm-owned root meta.json. Nested files with that name remain user content.
func CopyPayloadSyncTree(src, dst string) error {
	return copySyncTree(src, dst, true)
}

func copySyncTree(src, dst string, excludeMeta bool) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if st.Mode().IsRegular() {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(dst, data, st.Mode().Perm()); err != nil {
			return err
		}
		return os.Chmod(dst, st.Mode().Perm())
	}
	before, err := syncTree(src, excludeMeta)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	for _, n := range before.Nodes {
		to, err := SyncPath(dst, filepath.FromSlash(n.Path))
		if err != nil {
			return err
		}
		if n.Kind == "directory" {
			err = os.MkdirAll(to, fs.FileMode(n.Mode))
			if err == nil {
				err = os.Chmod(to, fs.FileMode(n.Mode))
			}
		} else {
			var data []byte
			data, err = os.ReadFile(filepath.Join(src, filepath.FromSlash(n.Path)))
			if err == nil {
				err = os.WriteFile(to, data, fs.FileMode(n.Mode))
			}
		}
		if err != nil {
			return err
		}
	}
	after, err := syncTree(src, excludeMeta)
	if err != nil {
		return err
	}
	copied, err := syncTree(dst, excludeMeta)
	if err != nil {
		return err
	}
	if before.Digest != after.Digest || before.Digest != copied.Digest {
		return fmt.Errorf("stale_input: source changed while copying")
	}
	return nil
}
