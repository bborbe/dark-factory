// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("Runner", func() {
	var (
		tempDir          string
		promptsDir       string
		specsDir         string
		manager          *mocks.Manager
		locker           *mocks.Locker
		watcher          *mocks.Watcher
		processor        *mocks.Processor
		server           *mocks.Server
		containerChecker *mocks.ContainerChecker
		ctx              context.Context
		cancel           context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "runner-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")

		manager = &mocks.Manager{}
		locker = &mocks.Locker{}
		watcher = &mocks.Watcher{}
		processor = &mocks.Processor{}
		server = &mocks.Server{}
		containerChecker = &mocks.ContainerChecker{}

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
			manager,
			locker,
			watcher,
			processor,
			server,
			nil, // no reviewPoller
			nil, // no specWatcher
			"",
			containerChecker,
			notifier.NewMultiNotifier(),
			&mocks.SpecSlugMigrator{},
			libtime.NewCurrentDateTime(),
			&mocks.FileMover{},
		)
	}

	It("should acquire and release lock", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		// Make watcher, processor, and server return immediately
		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
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
		Expect(locker.AcquireCallCount()).To(Equal(1))
		Expect(locker.ReleaseCallCount()).To(Equal(1))
	})

	It("should process executing prompts on startup", func() {
		inProgressDir := filepath.Join(promptsDir, "in-progress")
		Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)
		// No executing files — containerChecker.IsRunning not called
		containerChecker.IsRunningReturns(false, nil)

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
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

		// No executing prompts in dir — containerChecker should not be called
		Expect(containerChecker.IsRunningCallCount()).To(Equal(0))
	})

	It("should normalize filenames only in queue directory, not inbox", func() {
		inboxDir := filepath.Join(promptsDir, "inbox")
		queueDir := filepath.Join(promptsDir, "in-progress")

		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(inboxDir, queueDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify NormalizeFilenames was called only once (for queue, not inbox)
		Expect(manager.NormalizeFilenamesCallCount()).To(Equal(1))

		// Verify it was called with the in-progress directory
		_, dir := manager.NormalizeFilenamesArgsForCall(0)
		Expect(dir).To(Equal(queueDir))
	})

	It("should run watcher and processor in parallel", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		watcherCalled := make(chan struct{})
		processorCalled := make(chan struct{})

		watcher.WatchStub = func(ctx context.Context) error {
			close(watcherCalled)
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			close(processorCalled)
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
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
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
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

		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, context.DeadlineExceeded)

		r := newTestRunner(inboxDir, inProgressDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("normalize queue filenames"))

		// Verify lock was still released
		Expect(locker.ReleaseCallCount()).To(Equal(1))
	})

	It("should log renamed files during normalization", func() {
		inboxDir := filepath.Join(promptsDir, "inbox")
		inProgressDir := filepath.Join(promptsDir, "in-progress")

		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)

		// Simulate a rename during normalization
		manager.NormalizeFilenamesStub = func(ctx context.Context, dir string) ([]prompt.Rename, error) {
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

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		server.ListenAndServeStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		r := newTestRunner(inboxDir, inProgressDir, filepath.Join(promptsDir, "completed"))

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify normalization was called
		Expect(manager.NormalizeFilenamesCallCount()).To(Equal(1))
	})

	It("should not start server when server is nil", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
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
			manager,
			locker,
			watcher,
			processor,
			nil, // No server
			nil, // no reviewPoller
			nil, // no specWatcher
			"",
			containerChecker,
			notifier.NewMultiNotifier(),
			&mocks.SpecSlugMigrator{},
			libtime.NewCurrentDateTime(),
			&mocks.FileMover{},
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Verify watcher and processor were called
		Expect(watcher.WatchCallCount()).To(Equal(1))
		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	It("should include reviewPoller in run loop when non-nil", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		mockReviewPoller := &mocks.ReviewPoller{}
		pollerCalled := make(chan struct{})

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
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
			manager,
			locker,
			watcher,
			processor,
			nil, // No server
			mockReviewPoller,
			nil, // no specWatcher
			"",
			containerChecker,
			notifier.NewMultiNotifier(),
			&mocks.SpecSlugMigrator{},
			libtime.NewCurrentDateTime(),
			&mocks.FileMover{},
		)

		go func() {
			_ = r.Run(ctx)
		}()

		Eventually(pollerCalled, 1*time.Second).Should(BeClosed())

		cancel()
	})

	It("should not include reviewPoller in run loop when nil", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)

		watcher.WatchStub = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}
		processor.ProcessStub = func(ctx context.Context) error {
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
			manager,
			locker,
			watcher,
			processor,
			nil, // No server
			nil, // no reviewPoller
			nil, // no specWatcher
			"",
			containerChecker,
			notifier.NewMultiNotifier(),
			&mocks.SpecSlugMigrator{},
			libtime.NewCurrentDateTime(),
			&mocks.FileMover{},
		)

		runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer runCancel()

		err := r.Run(runCtx)
		Expect(err).To(BeNil())

		// Watcher and processor ran
		Expect(watcher.WatchCallCount()).To(Equal(1))
		Expect(processor.ProcessCallCount()).To(Equal(1))
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

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
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
				manager,
				locker,
				watcher,
				processor,
				server,
				nil,
				nil,
				"",
				containerChecker,
				notifier.NewMultiNotifier(),
				&mocks.SpecSlugMigrator{},
				libtime.NewCurrentDateTime(),
				&mocks.FileMover{},
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

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
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

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
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

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)
			// Stub Load to return nil for non-executing files
			manager.LoadReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
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

	Describe("resumeOrResetExecuting", func() {
		It("fires no notification and does no reset when no executing prompts exist", func() {
			inProgressDir := filepath.Join(promptsDir, "in-progress")
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

			fakeNotifier := &mocks.Notifier{}

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := runner.NewRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "logs"),
				filepath.Join(specsDir, "inbox"),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				watcher,
				processor,
				nil,
				nil,
				nil,
				"test-project",
				containerChecker,
				fakeNotifier,
				&mocks.SpecSlugMigrator{},
				libtime.NewCurrentDateTime(),
				&mocks.FileMover{},
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
			Expect(containerChecker.IsRunningCallCount()).To(Equal(0))
		})

		It("leaves prompt in executing state when container is still running", func() {
			inProgressDir := filepath.Join(promptsDir, "in-progress")
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

			stuckPromptPath := filepath.Join(inProgressDir, "001-stuck.md")
			Expect(
				os.WriteFile(
					stuckPromptPath,
					[]byte("---\nstatus: executing\ncontainer: proj-001-stuck\n---\ncontent"),
					0600,
				),
			).To(Succeed())

			fakeNotifier := &mocks.Notifier{}

			pf := prompt.NewPromptFile(
				stuckPromptPath,
				prompt.Frontmatter{
					Status:    string(prompt.ExecutingPromptStatus),
					Container: "proj-001-stuck",
				},
				[]byte("content"),
				libtime.NewCurrentDateTime(),
			)
			manager.LoadReturns(pf, nil)
			containerChecker.IsRunningReturns(true, nil)

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := runner.NewRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "logs"),
				filepath.Join(specsDir, "inbox"),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				watcher,
				processor,
				nil,
				nil,
				nil,
				"test-project",
				containerChecker,
				fakeNotifier,
				&mocks.SpecSlugMigrator{},
				libtime.NewCurrentDateTime(),
				&mocks.FileMover{},
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// Container running — no notification, prompt not reset
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(0))
			Expect(containerChecker.IsRunningCallCount()).To(Equal(1))
			_, name := containerChecker.IsRunningArgsForCall(0)
			Expect(name).To(Equal("proj-001-stuck"))

			// File should still have executing status (not reset)
			content, err := os.ReadFile(stuckPromptPath)
			Expect(err).To(BeNil())
			Expect(string(content)).To(ContainSubstring("status: executing"))
		})

		It(
			"resets prompt to approved and fires notification when container is not running",
			func() {
				inProgressDir := filepath.Join(promptsDir, "in-progress")
				Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

				stuckPromptPath := filepath.Join(inProgressDir, "001-stuck.md")
				Expect(
					os.WriteFile(
						stuckPromptPath,
						[]byte("---\nstatus: executing\ncontainer: proj-001-stuck\n---\ncontent"),
						0600,
					),
				).To(Succeed())

				fakeNotifier := &mocks.Notifier{}

				pf := prompt.NewPromptFile(
					stuckPromptPath,
					prompt.Frontmatter{
						Status:    string(prompt.ExecutingPromptStatus),
						Container: "proj-001-stuck",
					},
					[]byte("content"),
					libtime.NewCurrentDateTime(),
				)
				manager.LoadReturns(pf, nil)
				containerChecker.IsRunningReturns(false, nil)

				locker.AcquireReturns(nil)
				locker.ReleaseReturns(nil)
				manager.NormalizeFilenamesReturns(nil, nil)

				watcher.WatchStub = func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				}
				processor.ProcessStub = func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				}
				server.ListenAndServeStub = func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				}

				r := runner.NewRunner(
					filepath.Join(promptsDir, "inbox"),
					inProgressDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "logs"),
					filepath.Join(specsDir, "inbox"),
					filepath.Join(specsDir, "in-progress"),
					filepath.Join(specsDir, "completed"),
					filepath.Join(specsDir, "logs"),
					manager,
					locker,
					watcher,
					processor,
					nil,
					nil,
					nil,
					"test-project",
					containerChecker,
					fakeNotifier,
					&mocks.SpecSlugMigrator{},
					libtime.NewCurrentDateTime(),
					&mocks.FileMover{},
				)

				runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
				defer runCancel()

				err := r.Run(runCtx)
				Expect(err).To(BeNil())

				// Container gone — notification fired, prompt reset
				Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
				_, event := fakeNotifier.NotifyArgsForCall(0)
				Expect(event.EventType).To(Equal("stuck_container"))
				Expect(event.ProjectName).To(Equal("test-project"))
				Expect(event.PromptName).To(Equal("001-stuck.md"))

				// File should be reset to approved
				content, err := os.ReadFile(stuckPromptPath)
				Expect(err).To(BeNil())
				Expect(string(content)).To(ContainSubstring("status: approved"))
			},
		)

		It("resets prompt when container name is empty (treat as not running)", func() {
			inProgressDir := filepath.Join(promptsDir, "in-progress")
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())

			stuckPromptPath := filepath.Join(inProgressDir, "002-no-container.md")
			Expect(
				os.WriteFile(stuckPromptPath, []byte("---\nstatus: executing\n---\ncontent"), 0600),
			).To(Succeed())

			fakeNotifier := &mocks.Notifier{}

			pf := prompt.NewPromptFile(
				stuckPromptPath,
				prompt.Frontmatter{
					Status:    string(prompt.ExecutingPromptStatus),
					Container: "", // empty container name
				},
				[]byte("content"),
				libtime.NewCurrentDateTime(),
			)
			manager.LoadReturns(pf, nil)
			containerChecker.IsRunningReturns(false, nil)

			locker.AcquireReturns(nil)
			locker.ReleaseReturns(nil)
			manager.NormalizeFilenamesReturns(nil, nil)

			watcher.WatchStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			processor.ProcessStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}
			server.ListenAndServeStub = func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}

			r := runner.NewRunner(
				filepath.Join(promptsDir, "inbox"),
				inProgressDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "logs"),
				filepath.Join(specsDir, "inbox"),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				watcher,
				processor,
				nil,
				nil,
				nil,
				"test-project",
				containerChecker,
				fakeNotifier,
				&mocks.SpecSlugMigrator{},
				libtime.NewCurrentDateTime(),
				&mocks.FileMover{},
			)

			runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer runCancel()

			err := r.Run(runCtx)
			Expect(err).To(BeNil())

			// Empty container treated as not running — notification and reset
			Expect(fakeNotifier.NotifyCallCount()).To(Equal(1))
			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("stuck_container"))

			content, err := os.ReadFile(stuckPromptPath)
			Expect(err).To(BeNil())
			Expect(string(content)).To(ContainSubstring("status: approved"))
		})
	})
})
