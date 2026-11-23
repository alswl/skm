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

type heldRepositoryLockKey struct{}

// Context returns a context for synchronous operations under this held lock.
// Nested acquisition of the same repository is then a no-op; the owner remains
// responsible for releasing the original lock after all nested work returns.
func (l *Lock) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, heldRepositoryLockKey{}, l)
}

// DefaultLockWait bounds how long acquire blocks before failing.
const DefaultLockWait = 10 * time.Second

// LockName derives the temp-dir lock path for a repository root.
func LockName(root string) string {
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
		if physical, err := filepath.EvalSymlinks(absolute); err == nil {
			root = physical
		}
	}
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
	if ctx != nil {
		if held, ok := ctx.Value(heldRepositoryLockKey{}).(*Lock); ok && held != nil && held.file != nil && held.path == path {
			return &Lock{}, nil
		}
	}
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
