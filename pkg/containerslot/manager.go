// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package containerslot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/containerlock"
	"github.com/bborbe/dark-factory/pkg/executor"
)

//counterfeiter:generate -o ../../mocks/container-slot-manager.go --fake-name Manager . Manager

// Manager coordinates the per-host container concurrency limit.
type Manager interface {
	// Acquire blocks until a slot is available, then returns with the lock held
	// and an idempotent release function.
	Acquire(ctx context.Context) (release func(), err error)

	// ReleaseAfterStart releases the lock once the named container is running
	// (or after a 30s timeout). Spawns a goroutine; safe to call after Acquire.
	ReleaseAfterStart(ctx context.Context, containerName string, release func())
}

// NewManager creates a Manager that controls container slot concurrency.
// lock may be nil (no locking); checker may be nil (skips release-after-start).
func NewManager(
	lock containerlock.ContainerLock,
	counter executor.ContainerCounter,
	checker executor.ContainerChecker,
	maxContainers int,
	pollInterval time.Duration,
) Manager {
	return &manager{
		lock:          lock,
		counter:       counter,
		checker:       checker,
		maxContainers: maxContainers,
		pollInterval:  pollInterval,
	}
}

type manager struct {
	lock          containerlock.ContainerLock
	counter       executor.ContainerCounter
	checker       executor.ContainerChecker
	maxContainers int
	pollInterval  time.Duration
}

// Acquire acquires a container slot, blocking until one is available.
// On success, returns with the lock held and an idempotent release function.
func (m *manager) Acquire(ctx context.Context) (func(), error) {
	if m.lock == nil {
		if err := m.waitForSlot(ctx); err != nil {
			return func() {}, errors.Wrap(ctx, err, "wait for container slot")
		}
		return func() {}, nil
	}

	for {
		if err := m.lock.Acquire(ctx); err != nil {
			return func() {}, errors.Wrap(ctx, err, "acquire container lock")
		}

		// Idempotent release — safe to call multiple times.
		var once sync.Once
		releaseLock := func() {
			once.Do(func() { _ = m.lock.Release(ctx) })
		}

		if m.hasFreeSlot(ctx) {
			// Lock stays held; caller does docker run + ReleaseAfterStart.
			return releaseLock, nil
		}

		// No slot — release before sleeping so other daemons can proceed.
		releaseLock()
		slog.Info("waiting for container slot", "limit", m.maxContainers)
		select {
		case <-ctx.Done():
			return func() {}, errors.Wrapf(ctx, ctx.Err(), "wait for container slot cancelled")
		case <-time.After(m.pollInterval):
		}
	}
}

// ReleaseAfterStart spawns a goroutine that releases the lock once the named container is running.
func (m *manager) ReleaseAfterStart(ctx context.Context, containerName string, release func()) {
	if m.checker == nil {
		return
	}
	cc := m.checker
	go func() {
		defer release()
		_ = cc.WaitUntilRunning(ctx, containerName, 30*time.Second)
	}()
}

// waitForSlot polls until a slot is available (no lock held).
func (m *manager) waitForSlot(ctx context.Context) error {
	if m.maxContainers <= 0 {
		return nil
	}
	for {
		count, err := m.counter.CountRunning(ctx)
		if err != nil {
			slog.Warn("failed to count running containers, proceeding anyway", "error", err)
			return nil
		}
		if count < m.maxContainers {
			return nil
		}
		slog.Info(
			"waiting for container slot",
			"running", count,
			"limit", m.maxContainers,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.pollInterval):
		}
	}
}

// hasFreeSlot returns true when maxContainers is unlimited (<=0) or when
// the current running count is below maxContainers.
func (m *manager) hasFreeSlot(ctx context.Context) bool {
	if m.maxContainers <= 0 {
		return true
	}
	count, err := m.counter.CountRunning(ctx)
	if err != nil {
		slog.Warn("failed to count running containers, proceeding anyway", "error", err)
		return true
	}
	return count < m.maxContainers
}
