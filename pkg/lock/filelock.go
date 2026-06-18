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

//counterfeiter:generate -o ../../mocks/lock-dir-lock.go --fake-name LockDirLock . DirLock

// DirLock provides exclusive directory-scoped locking via an advisory flock on the
// directory fd. It serializes mutations to any file inside a status directory
// (e.g. prompts/in-progress/) so concurrent processes cannot interleave
// mid-mutation. The flock is per-fd and kernel-released when the fd is closed,
// so crash-release is automatic — no sidecar file is ever created on disk.
type DirLock interface {
	// Acquire attempts to acquire the lock, polling every 100ms until
	// the lock is acquired, the timeout elapses, or the context is cancelled.
	// Returns an error naming the directory and the elapsed timeout on failure.
	Acquire(ctx context.Context, timeout time.Duration) error
	// Release closes the directory fd, which drops the flock. Idempotent —
	// calling on an already-released lock returns nil. No file is removed.
	Release(ctx context.Context) error
}

// NewDirLock creates a DirLock that acquires an exclusive advisory flock on
// dirPath itself. dirPath must be a directory that exists at Acquire time;
// pass filepath.Dir(targetFile) to serialize mutations of files in that directory.
func NewDirLock(dirPath string) DirLock {
	return &dirLock{dirPath: dirPath}
}

type dirLock struct {
	dirPath string
	// mu guards fd against concurrent Acquire/Release on the same dirLock
	// instance. Without it, the Go memory model forbids the unsynchronized
	// read+write pattern even when the application logic appears serial.
	mu sync.Mutex
	fd *os.File
}

// Acquire implements DirLock.Acquire.
func (f *dirLock) Acquire(ctx context.Context, timeout time.Duration) error {
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
				return errors.Errorf(ctx, "lock acquire timeout: %s", f.dirPath)
			}
			if f.tryAcquire(ctx) == nil {
				return nil
			}
		}
	}
}

func (f *dirLock) tryAcquire(ctx context.Context) error {
	// Hold the mutex across the entire open+flock+assign so two goroutines
	// racing on the same dirLock cannot both succeed and overwrite f.fd,
	// orphaning the loser's fd (which remains flock'd but unreachable from
	// Release on this dirLock). The open+flock is a few syscalls; holding
	// the mutex over it is cheap and removes the race.
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fd != nil {
		// Another goroutine on this dirLock already acquired; treat as success.
		return nil
	}

	// #nosec G304 -- path is a caller-supplied directory path; open is O_RDONLY, no create, no write
	fd, err := os.OpenFile(f.dirPath, os.O_RDONLY, 0)
	if err != nil {
		return errors.Wrap(ctx, err, "open lock directory")
	}

	if err := syscall.Flock( //nolint:gosec // G115: File descriptor conversion is safe
		int(fd.Fd()),
		syscall.LOCK_EX|syscall.LOCK_NB,
	); err != nil {
		_ = fd.Close()
		return errors.Errorf(ctx, "flock failed: %v", err)
	}

	f.fd = fd
	return nil
}

// Release implements DirLock.Release.
//
// Ordering invariants:
//  1. Capture the fd locally; null out `f.fd` BEFORE any syscall so repeated
//     Release() calls are idempotent and a partial failure doesn't leave a
//     half-released state for the next Release attempt.
//  2. Close() before any further operations: closing the fd implicitly releases
//     all flock locks held on it (Linux/macOS man flock(2)). This is the only
//     step needed — no sidecar file exists to remove.
func (f *dirLock) Release(ctx context.Context) error {
	// Atomically take ownership of fd under the mutex so concurrent Release
	// calls cannot both capture the same fd (double-close).
	f.mu.Lock()
	fd := f.fd
	f.fd = nil
	f.mu.Unlock()

	if fd == nil {
		return nil
	}

	// Closing the fd drops the flock per man flock(2) on Linux and macOS.
	if err := fd.Close(); err != nil {
		return errors.Wrap(ctx, err, "close lock directory")
	}
	return nil
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
