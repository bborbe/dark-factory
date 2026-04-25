// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner_test

import (
	"context"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/runner"
)

var _ = Describe("OneShotRunner", func() {
	var (
		tempDir          string
		promptsDir       string
		specsDir         string
		manager          *mocks.RunnerPromptManager
		locker           *mocks.Locker
		processor        *mocks.Processor
		containerChecker *mocks.ContainerChecker
		ctx              context.Context
		cancel           context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "oneshot-runner-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		specsDir = filepath.Join(tempDir, "specs")

		manager = &mocks.RunnerPromptManager{}
		locker = &mocks.Locker{}
		processor = &mocks.Processor{}
		containerChecker = &mocks.ContainerChecker{}

		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	newTestOneShotRunner := func(inboxDir, inProgressDir, completedDir string) runner.OneShotRunner {
		return runner.NewOneShotRunner(
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
			processor,
			nil,
			libtime.NewCurrentDateTime(),
			containerChecker,
			false,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)
	}

	setupMocks := func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, nil)
		processor.ProcessReturns(nil)
	}

	It("should acquire and release lock", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(locker.AcquireCallCount()).To(Equal(1))
		Expect(locker.ReleaseCallCount()).To(Equal(1))
	})

	It("should call Process exactly once and return without blocking", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	It("should handle executing prompts on startup without calling ResetExecuting", func() {
		setupMocks()
		// inProgressDir is empty — no executing prompts, no container check
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		// ResetExecuting is not in the narrow PromptManager interface; selective logic used instead
		Expect(containerChecker.IsRunningCallCount()).To(Equal(0))
	})

	It("should return nil when queue is empty", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
	})

	It("should call Process (not ProcessQueue)", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	It("should return error when Process fails", func() {
		setupMocks()
		processor.ProcessReturns(context.DeadlineExceeded)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("deadline exceeded"))
	})

	It("should log renamed files during normalization", func() {
		setupMocks()
		manager.NormalizeFilenamesStub = func(manCtx context.Context, dir string) ([]prompt.Rename, error) {
			return []prompt.Rename{
				{
					OldPath: filepath.Join(promptsDir, "old.md"),
					NewPath: filepath.Join(promptsDir, "001-new.md"),
				},
			}, nil
		}

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
	})

	It(
		"should return context error when context is cancelled before Process runs",
		func() {
			setupMocks()
			// processor.Process returns ctx.Err() when context is already cancelled
			processor.ProcessStub = func(ctx context.Context) error {
				return ctx.Err()
			}

			r := newTestOneShotRunner(
				promptsDir,
				promptsDir,
				filepath.Join(promptsDir, "completed"),
			)

			cancel()

			err := r.Run(ctx)
			Expect(err).To(MatchError(context.Canceled))
		},
	)

	It("should return error when lock acquire fails", func() {
		locker.AcquireReturns(context.DeadlineExceeded)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))
		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("acquire lock"))
	})

	It("should return error when initial NormalizeFilenames fails", func() {
		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		manager.NormalizeFilenamesReturns(nil, context.DeadlineExceeded)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))
		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("normalize filenames"))
	})

	It("empty queue: exits cleanly via Process returning nil", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	It("blocked queue (gap in numbering): exits cleanly when Process returns nil", func() {
		// Regression test: old loop would spin forever on blocked queues.
		// Now Process is called once; the cancel-on-idle callback inside the real processor
		// ensures exit. The mock here just returns nil (simulating that path).
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	It("active queue with runnable prompt: drains via Process then exits", func() {
		setupMocks()
		// Simulate the real processor completing a prompt and returning via onIdle cancel.
		// The mock just returns nil to simulate clean exit after draining.
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(processor.ProcessCallCount()).To(Equal(1))
	})

	Describe("migrateQueueDir (oneshot)", func() {
		It("should rename old queue dir to in-progress when in-progress does not exist", func() {
			oldQueueDir := filepath.Join(tempDir, "queue")
			newInProgressDir := filepath.Join(tempDir, "in-progress")
			inboxDir := filepath.Join(tempDir, "inbox")
			completedDir := filepath.Join(tempDir, "completed")
			logsDir := filepath.Join(tempDir, "logs")
			Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())

			// Create old queue dir with a file; newInProgressDir does NOT exist
			Expect(os.MkdirAll(oldQueueDir, 0750)).To(Succeed())
			Expect(
				os.WriteFile(
					filepath.Join(oldQueueDir, "001-test.md"),
					[]byte("---\nstatus: queued\n---\ntest"),
					0600,
				),
			).To(Succeed())

			setupMocks()

			r := runner.NewOneShotRunner(
				inboxDir,
				newInProgressDir,
				completedDir,
				logsDir,
				filepath.Join(specsDir, "inbox"),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				processor,
				nil,
				libtime.NewCurrentDateTime(),
				containerChecker,
				false,
				&mocks.SpecSlugMigrator{},
				&mocks.FileMover{},
				nil,
			)

			err := r.Run(ctx)
			Expect(err).To(BeNil())

			// Old queue should be gone, new in-progress should have the file
			_, err = os.Stat(oldQueueDir)
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Stat(newInProgressDir)
			Expect(err).To(BeNil())
		})

		It("should skip migration when in-progress dir already exists", func() {
			oldQueueDir := filepath.Join(tempDir, "queue")
			newInProgressDir := filepath.Join(tempDir, "in-progress")
			inboxDir := filepath.Join(tempDir, "inbox")
			completedDir := filepath.Join(tempDir, "completed")
			logsDir := filepath.Join(tempDir, "logs")
			Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())

			// Create both old queue dir and new in-progress dir
			Expect(os.MkdirAll(oldQueueDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(newInProgressDir, 0750)).To(Succeed())

			setupMocks()

			r := runner.NewOneShotRunner(
				inboxDir,
				newInProgressDir,
				completedDir,
				logsDir,
				filepath.Join(specsDir, "inbox"),
				filepath.Join(specsDir, "in-progress"),
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				processor,
				nil,
				libtime.NewCurrentDateTime(),
				containerChecker,
				false,
				&mocks.SpecSlugMigrator{},
				&mocks.FileMover{},
				nil,
			)

			err := r.Run(ctx)
			Expect(err).To(BeNil())

			// Old queue should still exist (migration was skipped)
			_, err = os.Stat(oldQueueDir)
			Expect(err).To(BeNil())
		})
	})

})
