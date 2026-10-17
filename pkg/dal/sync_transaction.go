package dal

import (
	"crypto/sha256"
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
	Schema         int
	Entry          string
	Moves          []syncMove
	State          string
	BackupComplete bool
	Committed      bool
}

func TreeFingerprint(path string) (string, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	if st.Mode().IsRegular() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		// A file must never compare equal to a directory whose tree digest
		// happens to contain the same bytes. Transaction moves may include
		// .gitignore during a migration, so preserve the object kind here.
		return "file:" + hex.EncodeToString(sum[:]), nil
	}
	m, err := SyncTree(path)
	return m.Digest, err
}

// CommitSync requires the repository lock. Recovery uses fingerprints, not a
// phase counter, because a process can die between a rename and journal write.
func CommitSync(root, entry string, replacements []SyncReplacement) (retErr error) {
	if err := initSyncStore(root, entry != "migration"); err != nil {
		return err
	}
	return commitSync(root, entry, replacements, filepath.Join(".skm", "work", "transactions"))
}

// CommitFinalSync keeps a journal only for the duration of a final-format
// operation. It does not bootstrap the legacy record, baseline or cache store.
func CommitFinalSync(root, entry string, replacements []SyncReplacement) error {
	return commitSync(root, entry, replacements, filepath.Join(".skm", "temp", "transactions"))
}

func commitSync(root, entry string, replacements []SyncReplacement, journalRel string) (retErr error) {
	if err := RecoverSync(root, false); err != nil {
		return err
	}
	parent := filepath.Join(root, journalRel)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, "txn-")
	if err != nil {
		return err
	}
	// Successful updates deliberately do not create a history rollback point.
	j := syncJournal{Schema: 2, Entry: entry, State: "preparing"}
	published := false
	committed := false
	defer func() {
		if committed && retErr != nil {
			retErr = &SyncAcceptedError{Err: retErr}
		}
	}()

	defer func() {
		if !published {
			_ = os.RemoveAll(dir)
			pruneFinalTemp(root, dir)
		}
	}()
	for i, a := range replacements {
		if reservedSyncTarget(entry, a.Path) {
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
	// Every original tree is copied and verified before a single formal path is
	// renamed.  A killed process can therefore always be rolled back without
	// relying on which replacement happened to be first.
	for i, m := range j.Moves {
		if m.Before == "absent" {
			continue
		}
		path, err := SyncPath(root, m.Path)
		if err != nil {
			return err
		}
		backup := filepath.Join(dir, fmt.Sprintf("old-%d", i))
		if err := CopySyncTree(path, backup); err != nil {
			return err
		}
		if err := syncTreeFiles(backup); err != nil {
			return err
		}
		got, err := TreeFingerprint(backup)
		if err != nil || got != m.Before {
			if err != nil {
				return err
			}
			return fmt.Errorf("backup verification failed for %s", m.Path)
		}
	}
	j.BackupComplete = true
	j.State = "prepared"
	if err = WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
		return err
	}
	if err = SyncDirectory(parent); err != nil {
		return err
	}
	published = true
	j.State = "accepting"
	if err = WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
		return err
	}
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
			if err = os.Rename(path, filepath.Join(dir, fmt.Sprintf("removed-%d", i))); err != nil {
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
	j.State = "committed"
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
	for _, rel := range []string{filepath.Join(".skm", "work", "transactions"), filepath.Join(".skm", "temp", "transactions")} {
		if err := recoverSyncParent(root, rel, checkOnly); err != nil {
			return err
		}
	}
	return nil
}

func recoverSyncParent(root, rel string, checkOnly bool) error {
	parent, err := SyncPath(root, rel)
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
		if err != nil || j.Schema != 2 || !j.BackupComplete || len(j.Moves) == 0 {
			return fmt.Errorf("recovery_required: invalid journal %s", dir)
		}
		if err = validateSyncJournal(j); err != nil {
			return fmt.Errorf("recovery_required: %w", err)
		}
		if checkOnly {
			return fmt.Errorf("recovery_required: run skm sync recover (%s)", dir)
		}
		if !j.Committed && j.State == "rolling_back" {
			if checkOnly {
				return fmt.Errorf("recovery_required: run skm sync recover (%s)", dir)
			}
			if err := RollbackSync(root, j.Entry, false); err != nil {
				return err
			}
			continue
		}
		if !j.Committed && j.State == "rolled_back" {
			for _, m := range j.Moves {
				path, err := SyncPath(root, m.Path)
				if err != nil {
					return err
				}
				got, err := TreeFingerprint(path)
				if err != nil {
					return err
				}
				if got != m.Before {
					return fmt.Errorf("recovery_required: externally changed %s", m.Path)
				}
			}
			if err := finishSync(root, dir, j); err != nil {
				return err
			}
			continue
		}
		if !j.Committed {
			return fmt.Errorf("recovery_required: transaction %s needs explicit rollback", dir)
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
		if err = finishSync(root, dir, j); err != nil {
			return err
		}
	}
	return nil
}

// RollbackSync explicitly restores an interrupted, uncommitted transaction.
// It never guesses at an external edit: every slot must still be at its
// recorded before/after state (or at the documented rename gap) before the
// first path is changed. The durable journal remains until every slot is back
// at before, so a second interruption is safe to retry.
func RollbackSync(root, entry string, dryRun bool) error {
	for _, rel := range []string{filepath.Join(".skm", "work", "transactions"), filepath.Join(".skm", "temp", "transactions")} {
		found, err := rollbackSyncParent(root, rel, entry, dryRun)
		if found || err != nil {
			return err
		}
	}
	return fmt.Errorf("rollback_unavailable: no interrupted transaction for %s", entry)
}

func rollbackSyncParent(root, rel, entry string, dryRun bool) (bool, error) {
	parent, err := SyncPath(root, rel)
	if err != nil {
		return false, err
	}
	dirs, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(parent, d.Name())
		var j syncJournal
		if err := ReadSyncJSON(filepath.Join(dir, "journal.json"), &j); err != nil || j.Schema != 2 || !j.BackupComplete || j.Entry != entry || j.Committed || validateSyncJournal(j) != nil {
			continue
		}
		for i, m := range j.Moves {
			path, err := SyncPath(root, m.Path)
			if err != nil {
				return true, err
			}
			current, err := TreeFingerprint(path)
			if err != nil {
				return true, err
			}
			backup := filepath.Join(dir, fmt.Sprintf("old-%d", i))
			if m.Before != "absent" {
				got, err := TreeFingerprint(backup)
				if err != nil || got != m.Before {
					return true, fmt.Errorf("recovery_blocked: missing or corrupt backup for %s", m.Path)
				}
			}
			if current == m.Before || current == m.After {
				continue
			}
			// After moving a formal tree aside, the path is briefly absent. It
			// is safe only when the isolated original still verifies.
			removed, removedErr := TreeFingerprint(filepath.Join(dir, fmt.Sprintf("removed-%d", i)))
			if current == "absent" && m.Before != "absent" && removedErr == nil && removed == m.Before {
				continue
			}
			return true, fmt.Errorf("stale_input: %s changed after interrupted update", m.Path)
		}
		if dryRun {
			return true, nil
		}
		j.State = "rolling_back"
		if err := WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
			return true, err
		}
		if err := SyncDirectory(dir); err != nil {
			return true, err
		}
		for i, m := range j.Moves {
			path, err := SyncPath(root, m.Path)
			if err != nil {
				return true, err
			}
			current, err := TreeFingerprint(path)
			if err != nil {
				return true, err
			}
			if current == m.Before {
				continue
			}
			if m.Before == "absent" {
				if err := os.RemoveAll(path); err != nil {
					return true, err
				}
				continue
			}
			restore := filepath.Join(dir, fmt.Sprintf("restore-%d", i))
			if err := os.RemoveAll(restore); err != nil {
				return true, err
			}
			if err := CopySyncTree(filepath.Join(dir, fmt.Sprintf("old-%d", i)), restore); err != nil {
				return true, err
			}
			if current != "absent" {
				if err := os.Rename(path, filepath.Join(dir, fmt.Sprintf("discarded-%d", i))); err != nil {
					return true, err
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return true, err
			}
			if err := os.Rename(restore, path); err != nil {
				return true, err
			}
			if err := SyncDirectory(filepath.Dir(path)); err != nil {
				return true, err
			}
		}
		j.State = "rolled_back"
		if err := WriteSyncJSON(filepath.Join(dir, "journal.json"), j); err != nil {
			return true, err
		}
		if err := SyncDirectory(dir); err != nil {
			return true, err
		}
		if err := os.RemoveAll(dir); err != nil {
			return true, err
		}
		pruneFinalTemp(root, dir)
		if entry == "migration" {
			return true, CleanupMigrationBootstrap(root)
		}
		return true, nil
	}
	return false, nil
}

// CleanupMigrationBootstrap removes the temporary v2 root used only to host a
// migration journal before the migration accept point. It refuses to remove a
// root containing any versioned v2 state.
func CleanupMigrationBootstrap(root string) error {
	state := filepath.Join(root, ".skm")
	for _, name := range []string{"entries", "baselines"} {
		items, err := os.ReadDir(filepath.Join(state, name))
		if err == nil && len(items) > 0 {
			return fmt.Errorf("recovery_blocked: migration bootstrap contains %s", name)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.RemoveAll(state)
}

// InterruptedSyncEntries exposes only validated, unfinished transaction
// identities. Services uses it to offer an explicit rollback even when a
// partially replaced skill is no longer discoverable by the regular scanner.
type SyncTransactionStatus struct {
	ID        string
	Entry     string
	State     string
	Committed bool
}

// SyncTransactions returns the durable operation identities that recovery
// will inspect. It is read-only and rejects malformed journals rather than
// inventing an identity for a damaged transaction.
func SyncTransactions(root string) ([]SyncTransactionStatus, error) {
	var all []SyncTransactionStatus
	for _, rel := range []string{filepath.Join(".skm", "work", "transactions"), filepath.Join(".skm", "temp", "transactions")} {
		items, err := syncTransactionsParent(root, rel)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func syncTransactionsParent(root, rel string) ([]SyncTransactionStatus, error) {
	parent, err := SyncPath(root, rel)
	if err != nil {
		return nil, err
	}
	dirs, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]SyncTransactionStatus, 0, len(dirs))
	for _, d := range dirs {
		if !d.IsDir() {
			return nil, fmt.Errorf("recovery_required: unexpected transaction %s", d.Name())
		}
		var j syncJournal
		if err := ReadSyncJSON(filepath.Join(parent, d.Name(), "journal.json"), &j); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("recovery_required: invalid journal %s", d.Name())
		}
		if j.Schema != 2 || !j.BackupComplete || validateSyncJournal(j) != nil {
			return nil, fmt.Errorf("recovery_required: invalid journal %s", d.Name())
		}
		items = append(items, SyncTransactionStatus{ID: d.Name(), Entry: j.Entry, State: j.State, Committed: j.Committed})
	}
	return items, nil
}

func InterruptedSyncEntries(root string) ([]string, error) {
	items, err := SyncTransactions(root)
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if item.Committed || item.State == "rolled_back" {
			continue
		}
		if !seen[item.Entry] {
			entries = append(entries, item.Entry)
			seen[item.Entry] = true
		}
	}
	return entries, nil
}

func finishSync(root, dir string, j syncJournal) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	pruneFinalTemp(root, dir)
	return nil
}

func pruneFinalTemp(root, dir string) {
	temp := filepath.Join(root, ".skm", "temp")
	if filepath.Dir(filepath.Dir(dir)) != temp {
		return
	}
	_ = os.Remove(filepath.Dir(dir)) // only when no other transaction remains
	_ = os.Remove(temp)              // only when no pending operation remains
	_ = os.Remove(filepath.Join(root, ".skm"))
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
	if j.Schema != 2 || !j.Committed || j.Entry != entry || validateSyncJournal(j) != nil {
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
	if j.Schema != 2 || len(j.Moves) == 0 || !filepath.IsLocal(j.Entry) || j.Entry == "." {
		return fmt.Errorf("invalid transaction identity")
	}
	validHash := func(v string) bool {
		if v == "absent" {
			return true
		}
		v = strings.TrimPrefix(v, "file:")
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
		if top == ".git" || reservedSyncTarget(j.Entry, m.Path) {
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

func migrationStatePath(entry, path string) bool {
	if entry != "migration" {
		return false
	}
	switch path {
	case filepath.Join(".skm", "entries"), filepath.Join(".skm", "baselines"), filepath.Join(".skm", "cache"), filepath.Join(".skm", "work"), filepath.Join(".skm", "format.json"):
		return true
	}
	return false
}

func reservedSyncTarget(entry, path string) bool {
	if path == ".git" || strings.HasPrefix(path, ".git/") || path == ".skm" {
		return true
	}
	if !strings.HasPrefix(path, ".skm/") {
		return false
	}
	if migrationStatePath(entry, path) {
		return false
	}
	return path == filepath.Join(".skm", "temp") || strings.HasPrefix(path, filepath.Join(".skm", "temp")+string(filepath.Separator)) ||
		path == filepath.Join(".skm", "work") || strings.HasPrefix(path, filepath.Join(".skm", "work")+string(filepath.Separator)) ||
		path == filepath.Join(".skm", "cache") || strings.HasPrefix(path, filepath.Join(".skm", "cache")+string(filepath.Separator))
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
