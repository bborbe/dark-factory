// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package containerlock

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bborbe/errors"
)

const lockFileName = "container.lock"

const pollInterval = 100 * time.Millisecond

//counterfeiter:generate -o ../../mocks/container-lock.go --fake-name ContainerLock . ContainerLock

// ContainerLock serializes the check-and-start sequence across daemon instances
// so that the maxContainers limit is never exceeded.
type ContainerLock interface {
	// Acquire blocks until the lock is obtained or ctx is cancelled.
	Acquire(ctx context.Context) error
	// Release releases the lock.
	Release(ctx context.Context) error
}

// NewContainerLock creates a ContainerLock backed by a file at
// $HOME/.dark-factory/container.lock.
func NewContainerLock() (ContainerLock, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(context.Background(), err, "get user home dir")
	}
	lockDir := filepath.Join(homeDir, ".dark-factory")
	if err := os.MkdirAll(lockDir, 0750); err != nil { //nolint:gosec // 0750 is intentional for dir
		return nil, errors.Wrap(context.Background(), err, "create lock dir")
	}
	return &containerLock{
		lockPath: filepath.Join(lockDir, lockFileName),
	}, nil
}

// NewContainerLockFromPath creates a ContainerLock backed by the given lock file path.
// The parent directory must already exist. This is intended for testing.
func NewContainerLockFromPath(lockPath string) ContainerLock {
	return &containerLock{lockPath: lockPath}
}

// containerLock implements ContainerLock using syscall.Flock.
type containerLock struct {
	lockPath string
	fd       *os.File
}

// Acquire opens the lock file and obtains an exclusive flock, blocking until
// the lock is available or ctx is cancelled.
func (l *containerLock) Acquire(ctx context.Context) error {
	fd, err := os.OpenFile(l.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrapf(ctx, err, "open container lock file %s", l.lockPath)
	}

	for {
		// Non-blocking attempt
		flockErr := syscall.Flock(
			int(fd.Fd()), //nolint:gosec // G115: int conversion of uintptr fd is safe
			syscall.LOCK_EX|syscall.LOCK_NB,
		)
		if flockErr == nil {
			l.fd = fd
			return nil
		}
		if flockErr != syscall.EWOULDBLOCK {
			_ = fd.Close()
			return errors.Wrapf(ctx, flockErr, "flock container lock file")
		}

		// Lock is held by another instance; wait or respect cancellation
		select {
		case <-ctx.Done():
			_ = fd.Close()
			return errors.Wrapf(ctx, ctx.Err(), "acquire container lock cancelled")
		case <-time.After(pollInterval):
		}
	}
}

// Release unlocks and closes the lock file descriptor.
// The lock file itself is not removed because it is shared across processes.
func (l *containerLock) Release(ctx context.Context) error {
	if l.fd == nil {
		return nil
	}
	if err := syscall.Flock(
		int(l.fd.Fd()), //nolint:gosec // G115: int conversion of uintptr fd is safe
		syscall.LOCK_UN,
	); err != nil {
		return errors.Wrapf(ctx, err, "unlock container lock file")
	}
	if err := l.fd.Close(); err != nil {
		return errors.Wrapf(ctx, err, "close container lock file")
	}
	l.fd = nil
	return nil
}
