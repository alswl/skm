package dal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

func SyncKey(rel string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(rel))))
	return hex.EncodeToString(sum[:])
}
func SyncStateRel(rel string) string       { return filepath.Join(".skm-sync", "entries", SyncKey(rel)) }
func SyncLocalRel(kind, rel string) string { return filepath.Join(".skm-local", kind, SyncKey(rel)) }

func ReadSyncRecord(root, rel string) (*common.SyncRecord, error) {
	formatPath, err := SyncPath(root, filepath.Join(".skm-sync", "format.json"))
	if err != nil {
		return nil, err
	}
	var format struct {
		Schema int `json:"schema_version"`
	}
	if err = ReadSyncJSON(formatPath, &format); err != nil {
		return nil, err
	}
	if format.Schema != 1 {
		return nil, fmt.Errorf("unsupported sync format")
	}

	dir, err := SyncPath(root, SyncStateRel(rel))
	if err != nil {
		return nil, err
	}
	var rec common.SyncRecord
	if err = ReadSyncJSON(filepath.Join(dir, "record.json"), &rec); err != nil {
		return nil, err
	}
	if rec.Schema != 1 || rec.EntryPath != rel || rec.Name == "" || rec.Generation == 0 || (rec.Active != "upstream" && rec.Active != "fork") {
		return nil, fmt.Errorf("corrupt sync record for %s", rel)
	}
	if _, err = SyncPath(root, rec.EntryPath); err != nil {
		return nil, err
	}
	if rec.ForkPath != "" {
		if rec.ForkPath != filepath.Join("forks", SyncKey(rel)) {
			return nil, fmt.Errorf("corrupt fork path")
		}
		if _, err = SyncPath(root, rec.ForkPath); err != nil {
			return nil, err
		}
	}
	if rec.Active == "fork" && rec.ForkPath == "" {
		return nil, fmt.Errorf("active fork missing path")
	}
	return &rec, nil
}
func ReadSyncJSON(path string, v any) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("unsafe state file %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func WriteSyncJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteSyncFile(path, append(data, '\n'), 0600)
}
func WriteSyncFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(path))
}
func SyncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

// InitSyncStore owns only these reserved roots and one exact ignore entry.
func InitSyncStore(root string) error {
	if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
		out, gitErr := exec.Command("git", "-C", root, "ls-files", "--", ".skm-local").Output()
		if gitErr != nil {
			return fmt.Errorf("cannot verify local-state Git exclusion: %w", gitErr)
		}
		if len(out) > 0 {
			return fmt.Errorf(".skm-local contains Git-tracked data; remove it from version control before sync")
		}
	}

	for _, name := range []string{".skm-sync", ".skm-local"} {
		path, err := SyncPath(root, name)
		if err != nil {
			return err
		}
		if st, err := os.Stat(path); err == nil {
			if !st.IsDir() {
				return fmt.Errorf("reserved sync path %s", path)
			}
			var format struct {
				Schema int `json:"schema_version"`
			}
			if err = ReadSyncJSON(filepath.Join(path, "format.json"), &format); err != nil || format.Schema != 1 {
				return fmt.Errorf("unrecognized sync root %s", path)
			}
		} else if !os.IsNotExist(err) {
			return err
		} else {
			if err = os.MkdirAll(path, 0700); err != nil {
				return err
			}
			if err = WriteSyncJSON(filepath.Join(path, "format.json"), map[string]int{"schema_version": 1}); err != nil {
				return err
			}
		}
	}
	ignore, err := SyncPath(root, ".gitignore")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(ignore)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "/.skm-local/" {
			return nil
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return WriteSyncFile(ignore, append(data, []byte("/.skm-local/\n")...), 0644)
}
