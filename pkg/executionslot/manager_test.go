// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executionslot_test

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/executionslot"
)

const pollInterval = 10 * time.Millisecond

var _ = Describe("Manager", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Acquire — nil lock (no locking)", func() {
		It("returns immediately when maxContainers is 0", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, nil)
			m := executionslot.NewManager(nil, counter, nil, 0, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.CountRunningCallCount()).To(Equal(0))
		})

		It("returns immediately when count is below limit", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(2, nil)
			m := executionslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.CountRunningCallCount()).To(Equal(1))
		})

		It("waits and returns when slot becomes free", func() {
			var callCount int32
			counter := &mocks.ContainerCounter{}
			counter.CountRunningCalls(func(_ context.Context) (int, error) {
				n := int(atomic.AddInt32(&callCount, 1))
				if n == 1 {
					return 3, nil
				}
				return 2, nil
			})
			m := executionslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.CountRunningCallCount()).To(Equal(2))
		})

		It("returns ctx.Err() when context is cancelled while waiting", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(3, nil)
			m := executionslot.NewManager(nil, counter, nil, 3, pollInterval)
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, context.Canceled)).To(BeTrue())
		})

		It("proceeds when counter returns error", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, stderrors.New("docker error"))
			m := executionslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
		})
	})

	Describe("Acquire — with lock", func() {
		It("acquires lock and returns release when slot is free", func() {
			lock := &mocks.ContainerLock{}
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(2, nil)
			m := executionslot.NewManager(lock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(lock.AcquireCallCount()).To(Equal(1))
			Expect(lock.ReleaseCallCount()).To(Equal(0))
			// release is idempotent
			release()
			Expect(lock.ReleaseCallCount()).To(Equal(1))
			release()
			Expect(lock.ReleaseCallCount()).To(Equal(1))
		})

		It("retries when slot is full then free — key regression", func() {
			var callCount int32
			counter := &mocks.ContainerCounter{}
			counter.CountRunningCalls(func(_ context.Context) (int, error) {
				n := int(atomic.AddInt32(&callCount, 1))
				if n <= 2 {
					return 3, nil
				}
				return 2, nil
			})
			var events []string
			var eventsMu sync.Mutex
			appendEvent := func(e string) {
				eventsMu.Lock()
				events = append(events, e)
				eventsMu.Unlock()
			}
			lock := &mocks.ContainerLock{}
			lock.AcquireCalls(func(_ context.Context) error { appendEvent("A"); return nil })
			lock.ReleaseCalls(func(_ context.Context) error { appendEvent("R"); return nil })
			m := executionslot.NewManager(lock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(lock.AcquireCallCount()).To(Equal(3))
			Expect(lock.ReleaseCallCount()).To(Equal(2))
			Expect(events).To(Equal([]string{"A", "R", "A", "R", "A"}))
			release()
			Expect(lock.ReleaseCallCount()).To(Equal(3))
		})

		It("cancellation during slot-wait releases all acquired locks", func() {
			lock := &mocks.ContainerLock{}
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(3, nil)
			m := executionslot.NewManager(lock, counter, nil, 3, 50*time.Millisecond)
			go func() {
				time.Sleep(80 * time.Millisecond)
				cancel()
			}()
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, context.Canceled)).To(BeTrue())
			Expect(lock.ReleaseCallCount()).To(Equal(lock.AcquireCallCount()))
		})

		It("propagates acquire error", func() {
			lock := &mocks.ContainerLock{}
			lock.AcquireReturns(stderrors.New("flock denied"))
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, nil)
			m := executionslot.NewManager(lock, counter, nil, 3, pollInterval)
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("flock denied"))
			Expect(lock.ReleaseCallCount()).To(Equal(0))
		})

		It("tolerates counter error and keeps lock held", func() {
			lock := &mocks.ContainerLock{}
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, stderrors.New("docker ls failed"))
			m := executionslot.NewManager(lock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(lock.AcquireCallCount()).To(Equal(1))
			Expect(lock.ReleaseCallCount()).To(Equal(0))
		})
	})

	Describe("ReleaseAfterStart", func() {
		It("is a no-op when checker is nil", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, nil)
			m := executionslot.NewManager(nil, counter, nil, 0, pollInterval)
			released := false
			m.ReleaseAfterStart(ctx, "container-1", func() { released = true })
			// No goroutine spawned — release is never called.
			time.Sleep(20 * time.Millisecond)
			Expect(released).To(BeFalse())
		})

		It("fires release after WaitUntilRunning unblocks", func() {
			counter := &mocks.ContainerCounter{}
			counter.CountRunningReturns(0, nil)
			checker := &mocks.ExecutionChecker{}
			waitCh := make(chan struct{})
			checker.WaitUntilRunningCalls(func(_ context.Context, _ string, _ time.Duration) error {
				<-waitCh
				return nil
			})
			m := executionslot.NewManager(nil, counter, checker, 0, pollInterval)

			released := make(chan struct{})
			m.ReleaseAfterStart(ctx, "container-1", func() { close(released) })

			// Release is not yet fired.
			Consistently(released, 30*time.Millisecond).ShouldNot(BeClosed())

			// Unblock the checker.
			close(waitCh)

			Eventually(released, 200*time.Millisecond).Should(BeClosed())
		})
	})
})
