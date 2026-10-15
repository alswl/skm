package dal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// Lock is a cross-process repository lock held for the duration of a write
// operation. The lock file is named by a hash of the repository's absolute
// path and lives in the OS temp dir (FR-034).
type Lock struct {
	path string
	file *os.File
}

// DefaultLockWait bounds how long acquire blocks before failing.
const DefaultLockWait = 10 * time.Second

// LockName derives the temp-dir lock path for a repository root.
func LockName(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(os.TempDir(), "skm-"+hex.EncodeToString(sum[:8])+".lock")
}

// AcquireLock takes the repository lock with a bounded wait and backoff. The
// caller must call Release.
func AcquireLock(ctx context.Context, root string) (*Lock, error) {
	return AcquireLockWait(ctx, root, DefaultLockWait)
}

// AcquireLockWait is AcquireLock with a configurable wait budget (testable).
func AcquireLockWait(ctx context.Context, root string, wait time.Duration) (*Lock, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if canonical, e := filepath.EvalSymlinks(abs); e == nil {
		abs = canonical
	}
	path := LockName(abs)
	// A separate suffix prevents treating an old O_EXCL lock as an advisory lock.
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("legacy repository lock %s exists; stop old skm processes and remove it before retrying", path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	path += ".flock"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		if ctx != nil && ctx.Err() != nil {
			_ = f.Close()
			return nil, ctx.Err()
		}
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &Lock{path: path, file: f}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("timed out waiting for repository lock %s", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Release closes the descriptor; keep the inode so concurrent waiters share it.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	l.file = nil
}
