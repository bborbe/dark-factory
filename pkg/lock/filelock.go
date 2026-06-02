// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lock

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/lock-file-lock.go --fake-name LockFileLock . FileLock

// FileLock provides exclusive file-based locking with a per-path lock file.
// Unlike Locker (which locks a project-wide .dark-factory.lock), FileLock is
// used to serialize mutations to individual files so that concurrent processes
// (e.g. a daemon and the doctor command) cannot corrupt mid-write.
type FileLock interface {
	// Acquire attempts to acquire the lock, polling every 100ms until
	// the lock is acquired, the timeout elapses, or the context is cancelled.
	// Returns an error naming the path and the elapsed timeout on failure.
	Acquire(ctx context.Context, timeout time.Duration) error
	// Release unlocks and removes the lock file. Idempotent — calling on an
	// already-released lock returns nil.
	Release(ctx context.Context) error
}

// NewFileLock creates a FileLock for the given target path.
// The lock file is <path>.lock in the same directory as path.
func NewFileLock(path string) FileLock {
	return &fileLock{lockPath: path + ".lock"}
}

type fileLock struct {
	lockPath string
	// mu guards fd against concurrent Acquire/Release on the same FileLock
	// instance. Without it, the Go memory model forbids the unsynchronized
	// read+write pattern even when the application logic appears serial.
	mu sync.Mutex
	fd *os.File
}

// Acquire implements FileLock.Acquire.
func (f *fileLock) Acquire(ctx context.Context, timeout time.Duration) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(timeout)
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx, ctx.Err(), "acquire lock cancelled")
		case <-ticker.C:
			if time.Now().After(deadline) {
				return errors.Errorf(ctx, "lock acquire timeout: %s", f.lockPath)
			}
			if f.tryAcquire(ctx) == nil {
				return nil
			}
		}
	}
}

func (f *fileLock) tryAcquire(ctx context.Context) error {
	// #nosec G304 -- path is derived from caller-controlled target path + ".lock" suffix
	fd, err := os.OpenFile(f.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(ctx, err, "open lock file")
	}

	if err := syscall.Flock( //nolint:gosec // G115: File descriptor conversion is safe
		int(fd.Fd()),
		syscall.LOCK_EX|syscall.LOCK_NB,
	); err != nil {
		_ = fd.Close()
		return errors.Errorf(ctx, "flock failed: %v", err)
	}

	f.mu.Lock()
	f.fd = fd
	f.mu.Unlock()
	return nil
}

// Release implements FileLock.Release.
//
// Ordering invariants:
//  1. Capture the fd locally; null out `f.fd` BEFORE any syscall so repeated
//     Release() calls are idempotent and a partial failure doesn't leave a
//     half-released state for the next Release attempt.
//  2. Close() before Flock(LOCK_UN): closing the fd implicitly releases all
//     flock locks held on it (Linux/macOS man flock(2)). If we Unlock first
//     and then Close fails, the lock IS released but the caller would see
//     "release succeeded but resource leaked". Closing first guarantees the
//     lock is gone regardless of subsequent step outcomes.
//  3. Only then attempt os.Remove on the lock file. By that point we are
//     certain we no longer hold the lock; another process can recreate the
//     same lock file safely.
func (f *fileLock) Release(ctx context.Context) error {
	// Atomically take ownership of fd under the mutex so concurrent Release
	// calls cannot both capture the same fd (double-close).
	f.mu.Lock()
	fd := f.fd
	f.fd = nil
	f.mu.Unlock()

	if fd == nil {
		return nil
	}

	// Close first (this implicitly releases the flock). If close fails the
	// lock is in an indeterminate state; surface the error to the caller.
	if err := fd.Close(); err != nil {
		return errors.Wrap(ctx, err, "close lock file")
	}

	// Lock-file cleanup is intentionally omitted to avoid the TOCTOU window
	// where another process could flock the same file between our Close and
	// our Remove — we'd then delete a live lock file belonging to that
	// process. Leaving the lock file in place is correct: flock holds are
	// per-fd, not per-inode, so a leftover lock file does not prevent the
	// next Acquire from succeeding. Disk cost is negligible (the file is
	// empty), and ops can `find . -name "*.lock" -delete` if cosmetic
	// cleanup is wanted.
	return nil
}

// FileLockPath returns the lock file path for a given target file path.
func FileLockPath(targetPath string) string {
	return targetPath + ".lock"
}

// EnsureDirExists creates the directory containing path with mode 0750 if it does not exist.
func EnsureDirExists(ctx context.Context, path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create directory")
	}
	return nil
}
