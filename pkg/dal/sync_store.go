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

// SyncForkRel maps the ordinary skills layout to an equally readable fork
// layout. Other managed layouts get a stable, collision-free location.
func SyncForkRel(rel string) string {
	clean := filepath.Clean(rel)
	if clean == "skills" {
		return "skills-fork"
	}
	if strings.HasPrefix(filepath.ToSlash(clean), "skills/") {
		return filepath.Join("skills-fork", strings.TrimPrefix(filepath.ToSlash(clean), "skills/"))
	}
	return filepath.Join("skills-fork", "_other", SyncKey(clean))
}
func SyncStateRel(rel string) string { return filepath.Join(".skm", "entries", SyncKey(rel)) }

// SyncBaselineRel names a versioned raw snapshot. The digest is data, never a
// caller-controlled directory path.
func SyncBaselineRel(digest string) (string, error) {
	if len(digest) != 64 {
		return "", fmt.Errorf("invalid baseline digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("invalid baseline digest")
	}
	return filepath.Join(".skm", "baselines", strings.ToLower(digest)), nil
}

// SyncCacheRel isolates a shared immutable Git checkout by canonical source
// identity and full commit. Callers still validate the tree before using it.
func SyncCacheRel(repository, revision string) (string, error) {
	if len(revision) != 40 && len(revision) != 64 {
		return "", fmt.Errorf("invalid cache revision")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return "", fmt.Errorf("invalid cache revision")
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(repository))))
	return filepath.Join(".skm", "cache", "repos", hex.EncodeToString(key[:]), strings.ToLower(revision)), nil
}

type syncCacheManifest struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Digest     string `json:"digest"`
}

// ReadGitCache returns a verified immutable tree. A malformed or incomplete
// cache is reported as unavailable so the caller may fetch it again; it is
// never treated as a valid historical source.
func ReadGitCache(root, repository, revision string) (string, error) {
	rel, err := SyncCacheRel(repository, revision)
	if err != nil {
		return "", err
	}
	dir, err := SyncPath(root, rel)
	if err != nil {
		return "", err
	}
	var manifest syncCacheManifest
	if err = ReadSyncJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return "", err
	}
	if manifest.Repository != repository || !strings.EqualFold(manifest.Revision, revision) || len(manifest.Digest) != 64 {
		return "", fmt.Errorf("corrupt git cache manifest")
	}
	tree := filepath.Join(dir, "tree")
	got, err := TreeFingerprint(tree)
	if err != nil || got != manifest.Digest {
		return "", fmt.Errorf("corrupt git cache tree")
	}
	return tree, nil
}

// StoreGitCache publishes a complete source tree under its immutable identity.
// An already valid cache wins; invalid cache content is replaced only after a
// separately staged and verified tree is ready.
func StoreGitCache(root, repository, revision, source string) (string, error) {
	if err := InitSyncStore(root); err != nil {
		return "", err
	}
	if cached, err := ReadGitCache(root, repository, revision); err == nil {
		return cached, nil
	}
	rel, err := SyncCacheRel(repository, revision)
	if err != nil {
		return "", err
	}
	dest, err := SyncPath(root, rel)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(dest)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, ".cache-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	tree := filepath.Join(stage, "tree")
	if err = CopySyncTree(source, tree); err != nil {
		return "", err
	}
	digest, err := TreeFingerprint(tree)
	if err != nil {
		return "", err
	}
	if err = WriteSyncJSON(filepath.Join(stage, "manifest.json"), syncCacheManifest{Repository: repository, Revision: revision, Digest: digest}); err != nil {
		return "", err
	}
	if err = SyncDirectory(stage); err != nil {
		return "", err
	}
	if err = os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = os.Rename(stage, dest); err != nil {
		return "", err
	}
	if err = SyncDirectory(parent); err != nil {
		return "", err
	}
	return filepath.Join(dest, "tree"), nil
}

// SyncLocalRel keeps callers from owning a second state root. All transient
// work is under .skm/work and is deliberately ignored by Git.
func SyncLocalRel(kind, rel string) string { return filepath.Join(".skm", "work", kind, SyncKey(rel)) }

func ReadSyncRecord(root, rel string) (*common.SyncRecord, error) {
	formatPath, err := SyncPath(root, filepath.Join(".skm", "format.json"))
	if err != nil {
		return nil, err
	}
	var format struct {
		Schema int `json:"schema_version"`
	}
	if err = ReadSyncJSON(formatPath, &format); err != nil {
		return nil, err
	}
	if format.Schema != 2 {
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
	if rec.Schema != 2 || rec.EntryPath != rel || rec.Name == "" || rec.Generation == 0 || (rec.Active != "upstream" && rec.Active != "fork") {
		return nil, fmt.Errorf("corrupt sync record for %s", rel)
	}
	if _, err = SyncPath(root, rec.EntryPath); err != nil {
		return nil, err
	}
	if rec.ForkPath != "" {
		if !filepath.IsLocal(rec.ForkPath) || !strings.HasPrefix(filepath.ToSlash(rec.ForkPath), "skills-fork/") {
			return nil, fmt.Errorf("corrupt fork path")
		}
		if _, err = SyncPath(root, rec.ForkPath); err != nil {
			return nil, err
		}
	}
	if rec.Active == "fork" && rec.ForkPath == "" {
		return nil, fmt.Errorf("active fork missing path")
	}
	if err := validateBaselineRef(rec.UpstreamBase); err != nil {
		return nil, fmt.Errorf("corrupt upstream baseline: %w", err)
	}
	if rec.ForkBase != nil {
		if err := validateBaselineRef(*rec.ForkBase); err != nil {
			return nil, fmt.Errorf("corrupt fork baseline: %w", err)
		}
	}
	if rec.Active == "fork" && rec.ForkBase == nil {
		return nil, fmt.Errorf("active fork missing baseline reference")
	}
	return &rec, nil
}

func validateBaselineRef(ref common.BaselineRef) error {
	// Records created before v2.1 used sidecar baseline trees without a ref.
	// They remain readable for the explicit migration/repair path.
	if ref.Kind == "" {
		return nil
	}
	if len(ref.RawDigest) != 64 {
		return fmt.Errorf("invalid raw digest")
	}
	if _, err := hex.DecodeString(ref.RawDigest); err != nil {
		return fmt.Errorf("invalid raw digest")
	}
	switch ref.Kind {
	case "git":
		if len(ref.Revision) != 40 && len(ref.Revision) != 64 {
			return fmt.Errorf("git baseline must use full commit")
		}
		if _, err := hex.DecodeString(ref.Revision); err != nil {
			return fmt.Errorf("invalid git revision")
		}
		if ref.SnapshotDigest != "" {
			return fmt.Errorf("git baseline has snapshot")
		}
	case "snapshot":
		if ref.Revision != "sha256:"+ref.RawDigest || ref.SnapshotDigest != ref.RawDigest {
			return fmt.Errorf("invalid snapshot reference")
		}
	default:
		return fmt.Errorf("unknown baseline kind")
	}
	return nil
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

// InitSyncStore owns the single v2 management root. Entries and non-rebuildable
// baselines stay versioned; caches and unfinished operations stay local.
func InitSyncStore(root string) error { return initSyncStore(root, true) }

// initSyncStore lets the migration transaction create its journal without
// editing .gitignore before that file has been backed up. All normal callers
// retain InitSyncStore's complete initialization behavior.
func initSyncStore(root string, updateIgnore bool) error {
	_, v2Err := os.Lstat(filepath.Join(root, ".skm"))
	if os.IsNotExist(v2Err) {
		for _, legacy := range []string{".skm-sync", ".skm-local"} {
			if _, err := os.Lstat(filepath.Join(root, legacy)); err == nil {
				return fmt.Errorf("migration_required: legacy %s exists; run skm sync migrate", legacy)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	} else if v2Err != nil {
		return v2Err
	}
	if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
		out, gitErr := exec.Command("git", "-C", root, "ls-files", "--", ".skm/cache", ".skm/work").Output()
		if gitErr != nil {
			return fmt.Errorf("cannot verify local-state Git exclusion: %w", gitErr)
		}
		if len(out) > 0 {
			return fmt.Errorf(".skm cache or work contains Git-tracked data; remove it from version control before sync")
		}
	}

	for _, name := range []string{".skm"} {
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
			if err = ReadSyncJSON(filepath.Join(path, "format.json"), &format); err != nil || format.Schema != 2 {
				return fmt.Errorf("unrecognized sync root %s", path)
			}
		} else if !os.IsNotExist(err) {
			return err
		} else {
			if err = os.MkdirAll(path, 0700); err != nil {
				return err
			}
			if err = WriteSyncJSON(filepath.Join(path, "format.json"), map[string]int{"schema_version": 2}); err != nil {
				return err
			}
		}
	}
	if !updateIgnore {
		return nil
	}
	ignore, err := SyncPath(root, ".gitignore")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(ignore)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	missing := make([]string, 0, 6)
	for _, required := range []string{"!/.skm/", "!/.skm/format.json", "!/.skm/entries/", "!/.skm/entries/**", "/.skm/cache/", "/.skm/work/"} {
		found := false
		for _, line := range strings.Split(string(data), "\n") {
			if line == required {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, required)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return WriteSyncFile(ignore, append(data, []byte(strings.Join(missing, "\n")+"\n")...), 0644)
}

// LegacySyncStore reports whether a repository needs the explicit v1 to v2
// migration. Reads may still inspect it; writes must not create a parallel v2
// store next to it.
func LegacySyncStore(root string) bool {
	for _, name := range []string{".skm-sync", ".skm-local", "forks"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// RemoveLegacySyncIgnore retires only the exact v1 management-root rules.
// It leaves unrelated user patterns and comments untouched.
func RemoveLegacySyncIgnore(root string) error {
	ignore, err := SyncPath(root, ".gitignore")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(ignore)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	kept, changed := stripLegacySyncIgnore(data)
	if !changed {
		return nil
	}
	return WriteSyncFile(ignore, kept, 0644)
}

// MigratedSyncIgnore produces the exact v2 ignore file that a migration
// accepts atomically with the old state roots. It retains all unrelated user
// lines, removes only known v1 roots, and supplies the v2 local-state rules.
func MigratedSyncIgnore(data []byte) ([]byte, bool) {
	kept, changed := stripLegacySyncIgnore(data)
	lines := strings.Split(string(kept), "\n")
	seen := make(map[string]bool)
	for _, line := range lines {
		seen[line] = true
	}
	for _, required := range []string{"!/.skm/", "!/.skm/format.json", "!/.skm/entries/", "!/.skm/entries/**", "/.skm/cache/", "/.skm/work/"} {
		if !seen[required] {
			lines = append(lines, required)
			seen[required] = true
			changed = true
		}
	}
	return []byte(strings.Join(lines, "\n")), changed
}

func stripLegacySyncIgnore(data []byte) ([]byte, bool) {
	legacy := map[string]bool{"/.skm-sync/": true, "/.skm-local/": true, "/forks/": true, ".skm-sync/": true, ".skm-local/": true, "forks/": true}
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if legacy[line] {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n")), changed
}
