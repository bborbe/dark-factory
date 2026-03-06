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
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("Runner", func() {
	var (
		tempDir       string
		promptsDir    string
		specsDir      string
		mockManager   *mocks.Manager
		mockLocker    *mocks.Locker
		mockWatcher   *mocks.Watcher
		mockProcessor *mocks.Processor
		mockServer    *mocks.Server
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

		specsDir = filepath.Join(tempDir, "specs")

		mockManager = &mocks.Manager{}
		mockLocker = &mocks.Locker{}
		mockWatcher = &mocks.Watcher{}
		mockProcessor = &mocks.Processor{}
		mockServer = &mocks.Server{}

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	// newTestRunner creates a Runner with sensible test defaults for the new 8-dir params.
	newTestRunner := func(inboxDir, inProgressDir, completedDir string) runner.Runner {
		return runner.NewRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			filepath.Join(specsDir, "in-progress"),
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
			mockServer,
			nil, // no reviewPoller
			nil, // no specWatcher
		)
	}

	It("should acquire and release lock", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		// Make watcher, processor, and server return immediately
		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

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
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify ResetExecuting was called
		Expect(mockManager.ResetExecutingCallCount()).To(Equal(1))
	})

	It("should normalize filenames only in queue directory, not inbox", func() {
		inboxDir := filepath.Join(promptsDir, "inbox")
		queueDir := filepath.Join(promptsDir, "in-progress")

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
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(inboxDir, queueDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify NormalizeFilenames was called only once (for queue, not inbox)
		Expect(mockManager.NormalizeFilenamesCallCount()).To(Equal(1))

		// Verify it was called with the in-progress directory
		_, dir := mockManager.NormalizeFilenamesArgsForCall(0)
		Expect(dir).To(Equal(queueDir))
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
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

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
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

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

	It("should return error when normalization fails", func() {
		inboxDir := filepath.Join(promptsDir, "inbox")
		inProgressDir := filepath.Join(promptsDir, "in-progress")

		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, context.DeadlineExceeded)

		r := newTestRunner(inboxDir, inProgressDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("normalize queue filenames"))

		// Verify lock was still released
		Expect(mockLocker.ReleaseCallCount()).To(Equal(1))
	})

	It("should log renamed files during normalization", func() {
		inboxDir := filepath.Join(promptsDir, "inbox")
		inProgressDir := filepath.Join(promptsDir, "in-progress")

		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)

		// Simulate a rename during normalization
		mockManager.NormalizeFilenamesStub = func(ctx context.Context, dir string) ([]prompt.Rename, error) {
			if dir == inProgressDir {
				return []prompt.Rename{
					{
						OldPath: filepath.Join(inProgressDir, "old-name.md"),
						NewPath: filepath.Join(inProgressDir, "001-new-name.md"),
					},
				}, nil
			}
			return nil, nil
		}

		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockServer.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(inboxDir, inProgressDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify normalization was called
		Expect(mockManager.NormalizeFilenamesCallCount()).To(Equal(1))
	})

	It("should not start server when server is nil", func() {
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
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			filepath.Join(specsDir, "in-progress"),
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
			nil, // No server
			nil, // no reviewPoller
			nil, // no specWatcher
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify watcher and processor were called
		Expect(mockWatcher.WatchCallCount()).To(Equal(1))
		Expect(mockProcessor.ProcessCallCount()).To(Equal(1))
	})

	It("should include reviewPoller in run loop when non-nil", func() {
		mockLocker.AcquireReturns(nil)
		mockLocker.ReleaseReturns(nil)
		mockManager.ResetExecutingReturns(nil)
		mockManager.NormalizeFilenamesReturns(nil, nil)

		mockReviewPoller := &mocks.ReviewPoller{}
		pollerCalled := make(chan struct{})

		mockWatcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockProcessor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		mockReviewPoller.RunStub = func(ctx context.Context) error {
			close(pollerCalled)
			<-ctx.Done()
			return nil
		}

		r := runner.NewRunner(
			promptsDir,
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			filepath.Join(specsDir, "in-progress"),
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
			nil, // No server
			mockReviewPoller,
			nil, // no specWatcher
		)

		go func() {
			_ = r.Run(ctx)
		}()

		Eventually(pollerCalled, 1*time.Second).Should(BeClosed())

		cancel()
	})

	It("should not include reviewPoller in run loop when nil", func() {
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
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			filepath.Join(specsDir, "in-progress"),
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			mockManager,
			mockLocker,
			mockWatcher,
			mockProcessor,
			nil, // No server
			nil, // no reviewPoller
			nil, // no specWatcher
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Watcher and processor ran
		Expect(mockWatcher.WatchCallCount()).To(Equal(1))
		Expect(mockProcessor.ProcessCallCount()).To(Equal(1))
	})

	Describe("createDirectories", func() {
		It("should create all eight lifecycle directories on startup", func() {
			inboxDir := filepath.Join(promptsDir, "inbox")
			inProgressDir := filepath.Join(promptsDir, "in-progress")
			completedDir := filepath.Join(promptsDir, "completed")
			logDir := filepath.Join(promptsDir, "logs")
			specsInboxDir := filepath.Join(specsDir, "inbox")
			specsInProgressDir := filepath.Join(specsDir, "in-progress")
			specsCompletedDir := filepath.Join(specsDir, "completed")
			specsLogDir := filepath.Join(specsDir, "logs")

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
			mockServer.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := runner.NewRunner(
				inboxDir,
				inProgressDir,
				completedDir,
				logDir,
				specsInboxDir,
				specsInProgressDir,
				specsCompletedDir,
				specsLogDir,
				mockManager,
				mockLocker,
				mockWatcher,
				mockProcessor,
				mockServer,
				nil,
				nil,
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// Verify all eight directories were created
			for _, dir := range []string{
				inboxDir, inProgressDir, completedDir, logDir,
				specsInboxDir, specsInProgressDir, specsCompletedDir, specsLogDir,
			} {
				_, statErr := os.Stat(dir)
				Expect(statErr).To(BeNil(), "expected directory to exist: "+dir)
			}
		})
	})

	Describe("migrateQueueDir", func() {
		It("should rename prompts/queue to prompts/in-progress when old dir exists", func() {
			oldQueueDir := filepath.Join(promptsDir, "queue")
			inProgressDir := filepath.Join(promptsDir, "in-progress")

			// Create the old queue directory with a file in it
			Expect(os.MkdirAll(oldQueueDir, 0750)).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(oldQueueDir, "001-test.md"),
					[]byte("---\nstatus: queued\n---\ntest"),
					0600,
				),
			).To(Succeed())

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
			mockServer.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := newTestRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// Old queue dir should be gone
			_, err = os.Stat(oldQueueDir)
			Expect(os.IsNotExist(err)).To(BeTrue(), "old queue dir should not exist")

			// New in-progress dir should exist with the file
			_, err = os.Stat(inProgressDir)
			Expect(err).To(BeNil(), "in-progress dir should exist")
			_, err = os.Stat(filepath.Join(inProgressDir, "001-test.md"))
			Expect(err).To(BeNil(), "file should have been migrated")
		})

		It("should be a no-op when old queue dir does not exist", func() {
			inProgressDir := filepath.Join(promptsDir, "in-progress")
			// Do NOT create the old queue dir

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
			mockServer.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := newTestRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// in-progress dir should be created fresh (by createDirectories)
			_, statErr := os.Stat(inProgressDir)
			Expect(statErr).To(BeNil())
		})

		It("should skip migration when in-progress dir already exists", func() {
			oldQueueDir := filepath.Join(promptsDir, "queue")
			inProgressDir := filepath.Join(promptsDir, "in-progress")

			// Create BOTH old queue and new in-progress
			Expect(os.MkdirAll(oldQueueDir, 0750)).To(Succeed())
			Expect(
				os.WriteFile(filepath.Join(oldQueueDir, "old-file.md"), []byte("old"), 0600),
			).To(Succeed())
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
			Expect(
				os.WriteFile(filepath.Join(inProgressDir, "new-file.md"), []byte("new"), 0600),
			).To(Succeed())

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
			mockServer.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := newTestRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// Old queue dir should still exist (migration was skipped)
			_, err = os.Stat(oldQueueDir)
			Expect(err).To(BeNil(), "old queue dir should still exist when migration skipped")

			// New in-progress dir should still have original file
			_, err = os.Stat(filepath.Join(inProgressDir, "new-file.md"))
			Expect(err).To(BeNil(), "in-progress dir should retain its files")
		})
	})
})
