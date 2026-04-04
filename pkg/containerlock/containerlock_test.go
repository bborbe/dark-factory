// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package containerlock_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/containerlock"
)

var _ = Describe("ContainerLock", func() {
	var (
		ctx     context.Context
		tmpDir  string
		lock    containerlock.ContainerLock
		lockErr error
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tmpDir, err = os.MkdirTemp("", "containerlock-test-*")
		Expect(err).NotTo(HaveOccurred())
		lockPath := filepath.Join(tmpDir, "container.lock")
		lock = containerlock.NewContainerLockFromPath(lockPath)
	})

	AfterEach(func() {
		if lock != nil {
			_ = lock.Release(ctx)
		}
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	})

	Describe("Acquire", func() {
		JustBeforeEach(func() {
			lockErr = lock.Acquire(ctx)
		})

		It("succeeds on first call", func() {
			Expect(lockErr).NotTo(HaveOccurred())
		})

		It("creates the lock file", func() {
			lockPath := filepath.Join(tmpDir, "container.lock")
			_, err := os.Stat(lockPath)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Release", func() {
		BeforeEach(func() {
			err := lock.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds without error", func() {
			Expect(lock.Release(ctx)).NotTo(HaveOccurred())
		})

		It("allows acquiring the lock again after release", func() {
			Expect(lock.Release(ctx)).NotTo(HaveOccurred())

			lockPath := filepath.Join(tmpDir, "container.lock")
			secondLock := containerlock.NewContainerLockFromPath(lockPath)
			Expect(secondLock.Acquire(ctx)).NotTo(HaveOccurred())
			Expect(secondLock.Release(ctx)).NotTo(HaveOccurred())
		})

		Context("when called without prior Acquire", func() {
			BeforeEach(func() {
				Expect(lock.Release(ctx)).NotTo(HaveOccurred())
			})

			It("succeeds on a second release", func() {
				Expect(lock.Release(ctx)).NotTo(HaveOccurred())
			})
		})
	})

	Describe("mutual exclusion", func() {
		It("prevents two goroutines from holding the lock simultaneously", func() {
			lockPath := filepath.Join(tmpDir, "container.lock")

			var overlap atomic.Int32
			var wg sync.WaitGroup
			errors := make([]error, 2)

			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					l := containerlock.NewContainerLockFromPath(lockPath)
					if err := l.Acquire(ctx); err != nil {
						errors[idx] = err
						return
					}
					// Increment counter while holding lock
					n := overlap.Add(1)
					// n must never exceed 1 — would indicate two holders at once
					Expect(n).To(Equal(int32(1)))
					time.Sleep(20 * time.Millisecond)
					overlap.Add(-1)
					_ = l.Release(ctx)
				}(i)
			}
			wg.Wait()
			for _, err := range errors {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("second Acquire blocks until first is released", func() {
			lockPath := filepath.Join(tmpDir, "container.lock")

			firstLock := containerlock.NewContainerLockFromPath(lockPath)
			Expect(firstLock.Acquire(ctx)).NotTo(HaveOccurred())

			secondAcquired := make(chan struct{})
			go func() {
				secondLock := containerlock.NewContainerLockFromPath(lockPath)
				_ = secondLock.Acquire(ctx)
				close(secondAcquired)
				_ = secondLock.Release(ctx)
			}()

			// Second goroutine should be blocked
			Consistently(secondAcquired, 50*time.Millisecond, 10*time.Millisecond).
				ShouldNot(BeClosed())

			// Release first; second should unblock
			Expect(firstLock.Release(ctx)).NotTo(HaveOccurred())
			Eventually(secondAcquired, 2*time.Second, 10*time.Millisecond).Should(BeClosed())
		})
	})

	Describe("context cancellation", func() {
		It("returns an error when ctx is cancelled while waiting", func() {
			lockPath := filepath.Join(tmpDir, "container.lock")

			// First holder keeps the lock
			firstLock := containerlock.NewContainerLockFromPath(lockPath)
			Expect(firstLock.Acquire(ctx)).NotTo(HaveOccurred())
			defer func() { _ = firstLock.Release(ctx) }()

			cancelCtx, cancel := context.WithCancel(ctx)
			errCh := make(chan error, 1)
			go func() {
				secondLock := containerlock.NewContainerLockFromPath(lockPath)
				errCh <- secondLock.Acquire(cancelCtx)
			}()

			// Cancel while waiting
			time.Sleep(50 * time.Millisecond)
			cancel()

			Eventually(errCh, 2*time.Second).Should(Receive(HaveOccurred()))
		})
	})

	Describe("Acquire/Release round-trip", func() {
		It("handles multiple acquire/release cycles on the same lock", func() {
			lockPath := filepath.Join(tmpDir, "container.lock")
			l := containerlock.NewContainerLockFromPath(lockPath)
			for i := 0; i < 3; i++ {
				Expect(l.Acquire(ctx)).NotTo(HaveOccurred())
				Expect(l.Release(ctx)).NotTo(HaveOccurred())
			}
		})
	})
})
