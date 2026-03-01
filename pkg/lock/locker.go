// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/bborbe/errors"
)

const lockFileName = ".dark-factory.lock"

//counterfeiter:generate -o ../../mocks/locker.go --fake-name Locker . Locker

// Locker provides exclusive access control to prevent concurrent dark-factory instances.
type Locker interface {
	Acquire(ctx context.Context) error
	Release(ctx context.Context) error
}

// locker implements file-based locking using flock.
type locker struct {
	lockPath string
	fd       *os.File
}

// NewLocker creates a new Locker for the specified directory.
func NewLocker(dir string) Locker {
	return &locker{
		lockPath: filepath.Join(dir, lockFileName),
	}
}

// Acquire attempts to acquire an exclusive lock on the lock file.
// Returns an error if another instance is already running.
func (l *locker) Acquire(ctx context.Context) error {
	// Create the lock file (or open if exists)
	fd, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(ctx, err, "open lock file")
	}

	// Try to acquire exclusive lock (non-blocking)
	if err := syscall.Flock( //nolint:gosec // G115: File descriptor conversion is safe
		int(fd.Fd()),
		syscall.LOCK_EX|syscall.LOCK_NB,
	); err != nil {
		// Lock failed - another instance is running
		_ = fd.Close()

		// Try to read the PID from the lock file
		pid, readErr := l.readPID()
		if readErr == nil && pid > 0 {
			return errors.Errorf(ctx, "another instance is already running (pid %d)", pid)
		}
		return errors.Errorf(ctx, "another instance is already running")
	}

	// Write our PID to the lock file
	if err := l.writePID(ctx, fd); err != nil {
		_ = fd.Close()
		return errors.Wrap(ctx, err, "write pid to lock file")
	}

	// Keep file descriptor open to hold the lock
	l.fd = fd
	return nil
}

// Release releases the lock and removes the lock file.
func (l *locker) Release(ctx context.Context) error {
	if l.fd == nil {
		return nil
	}

	// Unlock the file
	if err := syscall.Flock( //nolint:gosec // G115: File descriptor conversion is safe
		int(l.fd.Fd()),
		syscall.LOCK_UN,
	); err != nil {
		return errors.Wrap(ctx, err, "unlock file")
	}

	// Close the file descriptor
	if err := l.fd.Close(); err != nil {
		return errors.Wrap(ctx, err, "close lock file")
	}
	l.fd = nil

	// Remove the lock file
	if err := os.Remove(l.lockPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(ctx, err, "remove lock file")
	}

	return nil
}

// readPID reads the PID from the lock file.
func (l *locker) readPID() (int, error) {
	data, err := os.ReadFile(l.lockPath)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// writePID writes the current process PID to the file.
func (l *locker) writePID(ctx context.Context, fd *os.File) error {
	pid := os.Getpid()
	// Truncate and seek to start (errors ignored as they should not fail on regular files)
	_ = fd.Truncate(0)
	_, _ = fd.Seek(0, 0)
	_, err := fmt.Fprintf(fd, "%d\n", pid)
	if err != nil {
		return errors.Wrap(ctx, err, "write pid")
	}
	return fd.Sync()
}
