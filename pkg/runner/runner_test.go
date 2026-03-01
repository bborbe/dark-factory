// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("Runner", func() {
	var (
		tempDir       string
		promptsDir    string
		mockManager   *mocks.Manager
		mockLocker    *mocks.Locker
		mockWatcher   *mocks.Watcher
		mockProcessor *mocks.Processor
		ctx           context.Context
		cancel        context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "runner-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		mockManager = &mocks.Manager{}
		mockLocker = &mocks.Locker{}
		mockWatcher = &mocks.Watcher{}
		mockProcessor = &mocks.Processor{}

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("should acquire and release lock", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		// Make watcher and processor return immediately
		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
		)

		// Run with timeout
		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify lock was acquired and released
		Expect(mockLocker.AcquireCallCount()).To(Equal(1))
		Expect(mockLocker.ReleaseCallCount()).To(Equal(1))
	})

	It("should reset executing prompts on startup", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify ResetExecuting was called
		Expect(mockManager.ResetExecutingCallCount()).To(Equal(1))
	})

	It("should normalize filenames on startup", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify NormalizeFilenames was called
		Expect(mockManager.NormalizeFilenamesCallCount()).To(Equal(1))
	})

	It("should run watcher and processor in parallel", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		watcherCalled := make(chan struct{})
		processorCalled := make(chan struct{})

		mockWatcher.WatchStub = func(ctx context.Context) error {
			close(watcherCalled)
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			close(processorCalled)
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
		)

		go func() {
			_ = r.Run(ctx)
		}()

		// Wait for both to be called
		Eventually(watcherCalled, 1*time.Second).Should(BeClosed())
		Eventually(processorCalled, 1*time.Second).Should(BeClosed())

		cancel()
	})

	It("should stop both goroutines on context cancel", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
		)

		runCtx, runCancel := context.WithCancel(ctx)

		errCh := make(chan error, 1)
		go func() {
			errCh <- r.Run(runCtx)
		}()

		// Let it run briefly
		time.Sleep(200 * time.Millisecond)

		// Cancel and wait for clean exit
		runCancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("runner did not exit after context cancel")
		}
	})
})
