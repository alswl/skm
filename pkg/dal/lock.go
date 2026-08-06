package dal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	path := LockName(root)
	deadline := time.Now().Add(wait)
	backoff := 25 * time.Millisecond
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return &Lock{path: path, file: f}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("cannot create lock %s: %w", path, err)
		}
		if ctx != nil && ctx.Err() != nil {
			return nil, fmt.Errorf("lock acquisition cancelled for %s: %w", path, ctx.Err())
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for repository lock %s", path)
		}
		time.Sleep(backoff)
		if backoff < time.Second {
			backoff *= 2
		}
	}
}

// Release removes the lock file. Safe to call multiple times.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
	_ = os.Remove(l.path)
	l.file = nil
}
