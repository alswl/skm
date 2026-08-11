package dal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// FileTransaction stages writes and can roll them back on failure (FR-034).
// Operations are recorded as an ordered log; Commit applies them, and on
// error the transaction undoes created links/moves and restores backups in
// reverse order.
type FileTransaction struct {
	ops []txOp
}

type txOp struct {
	kind   txKind
	dst    string
	backup string // where the replaced dst was moved before writing
}

type txKind int

const (
	txMove         txKind = iota // move staged content into place, backing up dst
	txLink                       // create a symlink at dst
	txRemove                     // remove dst (managed object)
	txBackupRemove               // back up and remove dst (force overwrite)
	txWrite                      // write a new regular file
)

// CopyFile records a regular-file copy at dst. The destination must not
// already exist; rollback removes the copied file.
func (t *FileTransaction) CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("transaction: prepare dir for %s: %w", dst, err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("transaction: refusing to copy over existing %s", dst)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("transaction: read %s: %w", src, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("transaction: stat %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("transaction: copy %s -> %s: %w", src, dst, err)
	}
	t.ops = append(t.ops, txOp{kind: txWrite, dst: dst})
	return nil
}

// MoveStage records that staged content at src will be moved to dst. If dst
// already exists it is moved to a backup so it can be restored on rollback.
// Cross-device renames fall back to copy-then-remove (temp dirs may live on a
// different volume than the repository).
func (t *FileTransaction) MoveStage(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("transaction: prepare dir for %s: %w", dst, err)
	}
	var backup string
	if _, err := os.Lstat(dst); err == nil {
		backup = backupName(dst)
		if err := os.Rename(dst, backup); err != nil {
			return fmt.Errorf("transaction: backup %s: %w", dst, err)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		if crossDevice(err) {
			if cErr := copyTreeFallback(src, dst); cErr == nil {
				_ = os.RemoveAll(src)
				t.ops = append(t.ops, txOp{kind: txMove, dst: dst, backup: backup})
				return nil
			}
		}
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		return fmt.Errorf("transaction: move %s -> %s: %w", src, dst, err)
	}
	t.ops = append(t.ops, txOp{kind: txMove, dst: dst, backup: backup})
	return nil
}

// CreateLink records a symlink creation at dst pointing at src.
func (t *FileTransaction) CreateLink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("transaction: prepare dir for %s: %w", dst, err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("transaction: refusing to link over existing %s", dst)
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("transaction: symlink %s -> %s: %w", dst, src, err)
	}
	t.ops = append(t.ops, txOp{kind: txLink, dst: dst})
	return nil
}

// Remove records that a managed object at dst will be removed (uninstall of a
// managed symlink/adapter). It performs the removal immediately and records
// the inverse (re-create) via Rollback needing the original link target.
func (t *FileTransaction) RemoveManaged(dst string) error {
	target, err := os.Readlink(dst)
	if err != nil {
		// Not a symlink; try plain remove only for directories we manage.
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("transaction: remove %s: %w", dst, err)
		}
		t.ops = append(t.ops, txOp{kind: txRemove, dst: dst})
		return nil
	}
	if err := os.Remove(dst); err != nil {
		return fmt.Errorf("transaction: remove %s: %w", dst, err)
	}
	// Recreate on rollback.
	t.ops = append(t.ops, txOp{kind: txRemove, dst: dst, backup: target})
	return nil
}

// BackupRemove moves dst to a backup so Rollback can restore it. Used when a
// --force install overwrites a conflicting user object.
func (t *FileTransaction) BackupRemove(dst string) error {
	backup := backupName(dst)
	if err := os.Rename(dst, backup); err != nil {
		return fmt.Errorf("transaction: backup remove %s: %w", dst, err)
	}
	t.ops = append(t.ops, txOp{kind: txBackupRemove, dst: dst, backup: backup})
	return nil
}

// Commit finalizes the transaction and removes backups of replaced targets
// (move/overwrite/convert), so no .skm-bak-* residue is left on disk.
func (t *FileTransaction) Commit() {
	for _, op := range t.ops {
		if op.backup != "" && (op.kind == txMove || op.kind == txBackupRemove) {
			_ = os.RemoveAll(op.backup)
		}
	}
	t.ops = nil
}

// Rollback undoes all recorded operations in reverse order, restoring backups
// for moved targets and recreating removed links.
func (t *FileTransaction) Rollback() error {
	var firstErr error
	ops := make([]txOp, len(t.ops))
	copy(ops, t.ops)
	// Reverse order of application.
	sort.SliceStable(ops, func(i, j int) bool { return i > j })
	for _, op := range ops {
		switch op.kind {
		case txMove:
			// Restore backup if we had one; otherwise remove the moved dst.
			if op.backup != "" {
				_ = os.Rename(op.backup, op.dst)
			} else {
				_ = os.RemoveAll(op.dst)
			}
		case txLink:
			_ = os.Remove(op.dst)
		case txRemove:
			// Recreate the removed managed link, if we recorded its target.
			if op.backup != "" {
				_ = os.Symlink(op.backup, op.dst)
			} else {
				_ = os.Remove(op.dst)
			}
		case txBackupRemove:
			_ = os.Rename(op.backup, op.dst)
		case txWrite:
			_ = os.Remove(op.dst)
		}
	}
	t.ops = nil
	return firstErr
}

// backupName returns a unique backup path for dst in the same directory.
func backupName(dst string) string {
	return fmt.Sprintf("%s.skm-bak-%d", dst, os.Getpid())
}

// crossDevice reports whether a rename error is a cross-device link error.
func crossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// copyTreeFallback copies a tree (used when a cross-device rename is not
// possible). Symlinks are preserved.
func copyTreeFallback(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
