package dal

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SyncReplacement struct{ Path, Source, Expected string }
type syncMove struct {
	Path   string
	Before string
	After  string
}
type syncJournal struct {
	Schema       int
	Entry        string
	Moves        []syncMove
	Committed    bool
	KeepRollback bool
}

func TreeFingerprint(path string) (string, error) {
	m, err := SyncTree(path)
	if os.IsNotExist(err) {
		return "absent", nil
	}
	return m.Digest, err
}

// CommitSync requires the repository lock. Recovery uses fingerprints, not a
// phase counter, because a process can die between a rename and journal write.
func CommitSync(root, entry string, replacements []SyncReplacement, keepRollback bool) (retErr error) {
	if err := InitSyncStore(root); err != nil {
		return err
	}
	if err := RecoverSync(root, false); err != nil {
		return err
	}
	parent := filepath.Join(root, ".skm-local", "transactions")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, "txn-")
	if err != nil {
		return err
	}
	j := syncJournal{Schema: 1, Entry: entry, KeepRollback: keepRollback}
	published := false
	committed := false
	defer func() {
		if published && retErr != nil {
			if recoveryErr := RecoverSync(root, false); recoveryErr != nil {
				retErr = fmt.Errorf("%w; recovery required: %v", retErr, recoveryErr)
			}
		}
		if committed && retErr != nil {
			retErr = &SyncAcceptedError{Err: retErr}
		}
	}()

	defer func() {
		if !published {
			_ = os.RemoveAll(dir)
		}
	}()
	for i, a := range replacements {
		if a.Path == ".git" || strings.HasPrefix(a.Path, ".git/") || a.Path == ".skm-local" || strings.HasPrefix(a.Path, ".skm-local/") {
			return fmt.Errorf("reserved transaction target")
		}
		for _, b := range replacements[:i] {
			if a.Path == b.Path || strings.HasPrefix(a.Path, b.Path+string(filepath.Separator)) || strings.HasPrefix(b.Path, a.Path+string(filepath.Separator)) {
				return fmt.Errorf("overlapping transaction targets")
			}
		}
	}
	for i, r := range replacements {
		path, err := SyncPath(root, r.Path)
		if err != nil {
			return err
		}
		before, err := TreeFingerprint(path)
		if err != nil {
			return err
		}
		if before != r.Expected {
			return fmt.Errorf("stale_input: %s changed", r.Path)
		}
		after := "absent"
		if r.Source != "" {
			dest := filepath.Join(dir, fmt.Sprintf("new-%d", i))
			if err = CopySyncTree(r.Source, dest); err != nil {
				return err
			}
			if err = syncTreeFiles(dest); err != nil {
				return err
			}
			after, err = TreeFingerprint(dest)
			if err != nil {
				return err
			}
		}
		j.Moves = append(j.Moves, syncMove{Path: r.Path, Before: before, After: after})
	}
	if err = WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
		return err
	}
	if err = SyncDirectory(parent); err != nil {
		return err
	}
	published = true
	for i, m := range j.Moves {
		path, err := SyncPath(root, m.Path)
		if err != nil {
			return err
		}
		observed, err := TreeFingerprint(path)
		if err != nil {
			return err
		}
		if observed != m.Before {
			return fmt.Errorf("stale_input: %s changed", m.Path)
		}
		if m.Before != "absent" {
			if err = os.Rename(path, filepath.Join(dir, fmt.Sprintf("old-%d", i))); err != nil {
				return err
			}
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if m.After != "absent" {
			if err = os.Rename(filepath.Join(dir, fmt.Sprintf("new-%d", i)), path); err != nil {
				return err
			}
		}
		if err = SyncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err = SyncDirectory(dir); err != nil {
			return err
		}
	}
	j.Committed = true
	if err = WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
		return err
	}
	committed = true
	return finishSync(root, dir, j)
}

func syncTreeFiles(root string) error {
	// File.Sync requests a full disk flush on macOS. Bound concurrency so
	// large skills can coalesce flushes without exhausting file descriptors.
	paths := make(chan string)
	errs := make(chan error, 1)
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			for path := range paths {
				if err := syncFile(path); err != nil {
					select {
					case errs <- err:
					default:
					}
				}
			}
		})
	}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		select {
		case err := <-errs:
			return err
		case paths <- path:
			return nil
		}
	})
	close(paths)
	workers.Wait()
	if err != nil {
		return err
	}
	select {
	case err := <-errs:
		return err
	default:
	}
	// Flush directory entries only after all file contents are durable,
	// working from children to parents before the journal is published.
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := SyncDirectory(dirs[i]); err != nil {
			return err
		}
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func RecoverSync(root string, checkOnly bool) error {
	parent, err := SyncPath(root, filepath.Join(".skm-local", "transactions"))
	if err != nil {
		return err
	}
	dirs, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			return fmt.Errorf("recovery_required: unexpected transaction %s", d.Name())
		}
		dir := filepath.Join(parent, d.Name())
		var j syncJournal
		err := ReadSyncJSON(filepath.Join(dir, "journal.json"), &j)
		if os.IsNotExist(err) {
			continue
		} // staging never published: no official mutation
		if err != nil || j.Schema != 1 || len(j.Moves) == 0 {
			return fmt.Errorf("recovery_required: invalid journal %s", dir)
		}
		if err = validateSyncJournal(j); err != nil {
			return fmt.Errorf("recovery_required: %w", err)
		}
		if checkOnly {
			return fmt.Errorf("recovery_required: run skm sync recover (%s)", dir)
		}
		// Validate the entire scene before changing any slot.
		for i, m := range j.Moves {
			path, err := SyncPath(root, m.Path)
			if err != nil {
				return err
			}
			got, err := TreeFingerprint(path)
			if err != nil {
				return err
			}
			old, err := TreeFingerprint(filepath.Join(dir, fmt.Sprintf("old-%d", i)))
			if err != nil {
				return err
			}
			if old != "absent" && old != m.Before {
				return fmt.Errorf("recovery_required: externally changed backup %s", m.Path)
			}
			if j.Committed {
				if got != m.After {
					return fmt.Errorf("recovery_required: externally changed %s", m.Path)
				}
			} else {
				if got != m.Before && got != m.After && got != "absent" {
					return fmt.Errorf("recovery_required: externally changed %s", m.Path)
				}
				if m.Before != "absent" && old != m.Before && got != m.Before {
					return fmt.Errorf("recovery_required: missing backup %s", m.Path)
				}
			}
		}
		if !j.Committed {
			for i := len(j.Moves) - 1; i >= 0; i-- {
				m := j.Moves[i]
				path, _ := SyncPath(root, m.Path)
				old := filepath.Join(dir, fmt.Sprintf("old-%d", i))
				backup, err := TreeFingerprint(old)
				if err != nil {
					return err
				}
				if backup == m.Before && m.Before != "absent" {
					if err = os.RemoveAll(path); err != nil {
						return err
					}
					if err = os.Rename(old, path); err != nil {
						return err
					}
				} else if m.Before == "absent" {
					if err = os.RemoveAll(path); err != nil {
						return err
					}
				}
				if err = SyncDirectory(filepath.Dir(path)); err != nil {
					return err
				}
			}
			if err = os.RemoveAll(dir); err != nil {
				return err
			}
		} else if err = finishSync(root, dir, j); err != nil {
			return err
		}
	}
	return nil
}

func finishSync(root, dir string, j syncJournal) error {
	if j.KeepRollback {
		dest, err := SyncPath(root, SyncLocalRel("rollback", j.Entry))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		// A committed journal remains authoritative until its old group is moved.
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
		if err := os.Rename(dir, dest); err != nil {
			return err
		}
		return SyncDirectory(filepath.Dir(dest))
	}
	return os.RemoveAll(dir)
}

// RollbackReplacements verifies the accepted group before exposing old sources.
func RollbackReplacements(root, entry string) ([]SyncReplacement, error) {
	dir, err := SyncPath(root, SyncLocalRel("rollback", entry))
	if err != nil {
		return nil, err
	}
	var j syncJournal
	if err = ReadSyncJSON(filepath.Join(dir, "journal.json"), &j); err != nil {
		return nil, fmt.Errorf("rollback_unavailable: %w", err)
	}
	if j.Schema != 1 || !j.Committed || j.Entry != entry || validateSyncJournal(j) != nil {
		return nil, fmt.Errorf("rollback_unavailable: invalid record")
	}
	var out []SyncReplacement
	for i, m := range j.Moves {
		path, err := SyncPath(root, m.Path)
		if err != nil {
			return nil, err
		}
		got, err := TreeFingerprint(path)
		if err != nil {
			return nil, err
		}
		if got != m.After {
			return nil, fmt.Errorf("stale_input: %s edited since update", m.Path)
		}
		source := ""
		if m.Before != "absent" {
			source = filepath.Join(dir, fmt.Sprintf("old-%d", i))
			hash, err := TreeFingerprint(source)
			if err != nil {
				return nil, err
			}
			if hash != m.Before {
				return nil, fmt.Errorf("rollback_unavailable: corrupt snapshot")
			}
		}
		out = append(out, SyncReplacement{Path: m.Path, Source: source, Expected: m.After})
	}
	return out, nil
}

func validateSyncJournal(j syncJournal) error {
	if j.Schema != 1 || len(j.Moves) == 0 || !filepath.IsLocal(j.Entry) || j.Entry == "." {
		return fmt.Errorf("invalid transaction identity")
	}
	validHash := func(v string) bool {
		if v == "absent" {
			return true
		}
		if len(v) != 64 {
			return false
		}
		_, err := hex.DecodeString(v)
		return err == nil
	}
	for i, m := range j.Moves {
		if !filepath.IsLocal(m.Path) || m.Path == "." || filepath.Clean(m.Path) != m.Path {
			return fmt.Errorf("invalid transaction path")
		}
		top := strings.Split(m.Path, string(filepath.Separator))[0]
		if top == ".git" || top == ".skm-local" || m.Path == ".skm-sync" {
			return fmt.Errorf("reserved transaction path")
		}
		if !validHash(m.Before) || !validHash(m.After) {
			return fmt.Errorf("invalid transaction fingerprint")
		}
		for _, other := range j.Moves[:i] {
			if other.Path == m.Path || strings.HasPrefix(other.Path, m.Path+string(filepath.Separator)) || strings.HasPrefix(m.Path, other.Path+string(filepath.Separator)) {
				return fmt.Errorf("overlapping transaction paths")
			}
		}
	}
	return nil
}

// SyncAcceptedError means the durable commit succeeded but local cleanup failed.
type SyncAcceptedError struct{ Err error }

func (e *SyncAcceptedError) Error() string {
	return "repository accepted; cleanup failed: " + e.Err.Error()
}
func (e *SyncAcceptedError) Unwrap() error { return e.Err }
func IsSyncAccepted(err error) bool {
	var accepted *SyncAcceptedError
	return errors.As(err, &accepted)
}
