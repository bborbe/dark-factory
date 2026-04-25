// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package containerslot_test

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/containerslot"
)

// stubContainerCounter is a minimal ContainerCounter for tests.
type stubContainerCounter struct {
	callCount int32
	fn        func(n int) (int, error)
}

func (s *stubContainerCounter) CountRunning(_ context.Context) (int, error) {
	n := int(atomic.AddInt32(&s.callCount, 1))
	return s.fn(n)
}

func (s *stubContainerCounter) calls() int {
	return int(atomic.LoadInt32(&s.callCount))
}

// fakeContainerLock is a test fake for containerlock.ContainerLock.
type fakeContainerLock struct {
	mu               sync.Mutex
	acquireCallCount int32
	releaseCallCount int32
	acquireErr       error
	acquireStub      func(context.Context) error
	releaseStub      func(context.Context) error
}

func (f *fakeContainerLock) Acquire(ctx context.Context) error {
	atomic.AddInt32(&f.acquireCallCount, 1)
	f.mu.Lock()
	stub := f.acquireStub
	err := f.acquireErr
	f.mu.Unlock()
	if stub != nil {
		return stub(ctx)
	}
	return err
}

func (f *fakeContainerLock) Release(ctx context.Context) error {
	atomic.AddInt32(&f.releaseCallCount, 1)
	f.mu.Lock()
	stub := f.releaseStub
	f.mu.Unlock()
	if stub != nil {
		return stub(ctx)
	}
	return nil
}

func (f *fakeContainerLock) acquireCalls() int {
	return int(atomic.LoadInt32(&f.acquireCallCount))
}

func (f *fakeContainerLock) releaseCalls() int {
	return int(atomic.LoadInt32(&f.releaseCallCount))
}

// fakeContainerChecker is a test fake for executor.ContainerChecker.
type fakeContainerChecker struct {
	// waitCh is closed by the test to unblock WaitUntilRunning.
	waitCh    chan struct{}
	callCount int32
}

func (f *fakeContainerChecker) IsRunning(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (f *fakeContainerChecker) WaitUntilRunning(
	_ context.Context,
	_ string,
	_ time.Duration,
) error {
	atomic.AddInt32(&f.callCount, 1)
	if f.waitCh != nil {
		<-f.waitCh
	}
	return nil
}

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
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 0, nil }}
			m := containerslot.NewManager(nil, counter, nil, 0, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.calls()).To(Equal(0))
		})

		It("returns immediately when count is below limit", func() {
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 2, nil }}
			m := containerslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.calls()).To(Equal(1))
		})

		It("waits and returns when slot becomes free", func() {
			counter := &stubContainerCounter{fn: func(n int) (int, error) {
				if n == 1 {
					return 3, nil
				}
				return 2, nil
			}}
			m := containerslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(counter.calls()).To(Equal(2))
		})

		It("returns ctx.Err() when context is cancelled while waiting", func() {
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 3, nil }}
			m := containerslot.NewManager(nil, counter, nil, 3, pollInterval)
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, context.Canceled)).To(BeTrue())
		})

		It("proceeds when counter returns error", func() {
			counter := &stubContainerCounter{fn: func(_ int) (int, error) {
				return 0, stderrors.New("docker error")
			}}
			m := containerslot.NewManager(nil, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
		})
	})

	Describe("Acquire — with lock", func() {
		It("acquires lock and returns release when slot is free", func() {
			fakeLock := &fakeContainerLock{}
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 2, nil }}
			m := containerslot.NewManager(fakeLock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(fakeLock.acquireCalls()).To(Equal(1))
			Expect(fakeLock.releaseCalls()).To(Equal(0))
			// release is idempotent
			release()
			Expect(fakeLock.releaseCalls()).To(Equal(1))
			release()
			Expect(fakeLock.releaseCalls()).To(Equal(1))
		})

		It("retries when slot is full then free — key regression", func() {
			counter := &stubContainerCounter{fn: func(n int) (int, error) {
				if n <= 2 {
					return 3, nil
				}
				return 2, nil
			}}
			var events []string
			var eventsMu sync.Mutex
			appendEvent := func(e string) {
				eventsMu.Lock()
				events = append(events, e)
				eventsMu.Unlock()
			}
			fakeLock := &fakeContainerLock{
				acquireStub: func(_ context.Context) error { appendEvent("A"); return nil },
				releaseStub: func(_ context.Context) error { appendEvent("R"); return nil },
			}
			m := containerslot.NewManager(fakeLock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeLock.acquireCalls()).To(Equal(3))
			Expect(fakeLock.releaseCalls()).To(Equal(2))
			Expect(events).To(Equal([]string{"A", "R", "A", "R", "A"}))
			release()
			Expect(fakeLock.releaseCalls()).To(Equal(3))
		})

		It("cancellation during slot-wait releases all acquired locks", func() {
			fakeLock := &fakeContainerLock{}
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 3, nil }}
			m := containerslot.NewManager(fakeLock, counter, nil, 3, 50*time.Millisecond)
			go func() {
				time.Sleep(80 * time.Millisecond)
				cancel()
			}()
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, context.Canceled)).To(BeTrue())
			Expect(fakeLock.releaseCalls()).To(Equal(fakeLock.acquireCalls()))
		})

		It("propagates acquire error", func() {
			fakeLock := &fakeContainerLock{acquireErr: stderrors.New("flock denied")}
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 0, nil }}
			m := containerslot.NewManager(fakeLock, counter, nil, 3, pollInterval)
			_, err := m.Acquire(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("flock denied"))
			Expect(fakeLock.releaseCalls()).To(Equal(0))
		})

		It("tolerates counter error and keeps lock held", func() {
			fakeLock := &fakeContainerLock{}
			counter := &stubContainerCounter{fn: func(_ int) (int, error) {
				return 0, stderrors.New("docker ls failed")
			}}
			m := containerslot.NewManager(fakeLock, counter, nil, 3, pollInterval)
			release, err := m.Acquire(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(release).NotTo(BeNil())
			Expect(fakeLock.acquireCalls()).To(Equal(1))
			Expect(fakeLock.releaseCalls()).To(Equal(0))
		})
	})

	Describe("ReleaseAfterStart", func() {
		It("is a no-op when checker is nil", func() {
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 0, nil }}
			m := containerslot.NewManager(nil, counter, nil, 0, pollInterval)
			released := false
			m.ReleaseAfterStart(ctx, "container-1", func() { released = true })
			// No goroutine spawned — release is never called.
			time.Sleep(20 * time.Millisecond)
			Expect(released).To(BeFalse())
		})

		It("fires release after WaitUntilRunning unblocks", func() {
			counter := &stubContainerCounter{fn: func(_ int) (int, error) { return 0, nil }}
			waitCh := make(chan struct{})
			checker := &fakeContainerChecker{waitCh: waitCh}
			m := containerslot.NewManager(nil, counter, checker, 0, pollInterval)

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
