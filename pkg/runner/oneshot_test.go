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
		manager          *mocks.Manager
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

		manager = &mocks.Manager{}
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
		processor.ProcessQueueReturns(nil)
	}

	It("should acquire and release lock", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(locker.AcquireCallCount()).To(Equal(1))
		Expect(locker.ReleaseCallCount()).To(Equal(1))
	})

	It("should call ProcessQueue and return without blocking", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(processor.ProcessQueueCallCount()).To(Equal(1))
	})

	It("should handle executing prompts on startup without calling ResetExecuting", func() {
		setupMocks()
		// inProgressDir is empty — no executing prompts, no container check
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		// ResetExecuting is no longer called — selective logic used instead
		Expect(manager.ResetExecutingCallCount()).To(Equal(0))
		Expect(containerChecker.IsRunningCallCount()).To(Equal(0))
	})

	It("should return nil when queue is empty", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
	})

	It("should not call Process (only ProcessQueue)", func() {
		setupMocks()
		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		Expect(processor.ProcessCallCount()).To(Equal(0))
		Expect(processor.ProcessQueueCallCount()).To(Equal(1))
	})

	It("should skip spec generation when specGenerator is nil", func() {
		setupMocks()
		// Create an approved spec file — it should be ignored when generator is nil
		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())
		specContent := "---\nstatus: approved\n---\n# Test Spec\n"
		Expect(
			os.WriteFile(
				filepath.Join(specInProgressDir, "001-spec.md"),
				[]byte(specContent),
				0600,
			),
		).To(Succeed())

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(processor.ProcessQueueCallCount()).To(Equal(1))
	})

	It("should skip specs that are not approved", func() {
		setupMocks()
		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

		// A spec in "prompted" status — should not trigger generation
		specContent := "---\nstatus: prompted\n---\n# Test Spec\n"
		Expect(
			os.WriteFile(
				filepath.Join(specInProgressDir, "001-spec.md"),
				[]byte(specContent),
				0600,
			),
		).To(Succeed())

		mockSpecGen := &mocks.SpecGenerator{}

		r := runner.NewOneShotRunner(
			promptsDir,
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			specInProgressDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			mockSpecGen,
			libtime.NewCurrentDateTime(),
			containerChecker,
			false,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(mockSpecGen.GenerateCallCount()).To(Equal(0))
		Expect(processor.ProcessQueueCallCount()).To(Equal(1))
	})

	It("should generate prompts from approved spec and loop until idle", func() {
		inboxDir := filepath.Join(tempDir, "inbox")
		inProgressDir := filepath.Join(tempDir, "in-progress")
		completedDir := filepath.Join(tempDir, "completed")
		logsDir := filepath.Join(tempDir, "logs")
		for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir} {
			Expect(os.MkdirAll(d, 0750)).To(Succeed())
		}

		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

		// Create an approved spec file
		specContent := "---\nstatus: approved\n---\n# My Spec\n"
		specFile := filepath.Join(specInProgressDir, "001-my-spec.md")
		Expect(os.WriteFile(specFile, []byte(specContent), 0600)).To(Succeed())

		// Mock generator: set spec to "prompted" and create a prompt in inbox
		mockSpecGen := &mocks.SpecGenerator{}
		mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
			// Update spec to prompted so it won't be picked up again
			Expect(
				os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600),
			).To(Succeed())
			// Create a prompt in inbox
			promptContent := "---\ntitle: generated prompt\nstatus: approved\n---\n# Prompt body\n"
			return os.WriteFile(
				filepath.Join(inboxDir, "001-gen-prompt.md"),
				[]byte(promptContent),
				0600,
			)
		}

		setupMocks()

		r := runner.NewOneShotRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			logsDir,
			filepath.Join(specsDir, "inbox"),
			specInProgressDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			mockSpecGen,
			libtime.NewCurrentDateTime(),
			containerChecker,
			true,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		// Generator called once for the approved spec
		Expect(mockSpecGen.GenerateCallCount()).To(Equal(1))
		// ProcessQueue called twice: once after generation (gen=1), once after idle check (gen=0, queued=0)
		Expect(processor.ProcessQueueCallCount()).To(Equal(2))

		// Verify prompt was moved from inbox to in-progress and approved
		_, statErr := os.Stat(filepath.Join(inProgressDir, "001-gen-prompt.md"))
		Expect(statErr).To(BeNil())
		_, statErr = os.Stat(filepath.Join(inboxDir, "001-gen-prompt.md"))
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})

	It("should handle empty inbox dir gracefully during approveInboxPrompts", func() {
		inboxDir := filepath.Join(tempDir, "inbox")
		inProgressDir := filepath.Join(tempDir, "in-progress")
		completedDir := filepath.Join(tempDir, "completed")
		logsDir := filepath.Join(tempDir, "logs")
		for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir} {
			Expect(os.MkdirAll(d, 0750)).To(Succeed())
		}

		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

		specContent := "---\nstatus: approved\n---\n# My Spec\n"
		specFile := filepath.Join(specInProgressDir, "001-my-spec.md")
		Expect(os.WriteFile(specFile, []byte(specContent), 0600)).To(Succeed())

		// Generator marks spec as prompted but creates no inbox files
		mockSpecGen := &mocks.SpecGenerator{}
		mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
			return os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600)
		}

		setupMocks()

		r := runner.NewOneShotRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			logsDir,
			filepath.Join(specsDir, "inbox"),
			specInProgressDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			mockSpecGen,
			libtime.NewCurrentDateTime(),
			containerChecker,
			false,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(mockSpecGen.GenerateCallCount()).To(Equal(1))
	})

	It("should skip non-.md files in inbox during approveInboxPrompts", func() {
		inboxDir := filepath.Join(tempDir, "inbox")
		inProgressDir := filepath.Join(tempDir, "in-progress")
		completedDir := filepath.Join(tempDir, "completed")
		logsDir := filepath.Join(tempDir, "logs")
		for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir} {
			Expect(os.MkdirAll(d, 0750)).To(Succeed())
		}

		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

		specContent := "---\nstatus: approved\n---\n# My Spec\n"
		specFile := filepath.Join(specInProgressDir, "001-my-spec.md")
		Expect(os.WriteFile(specFile, []byte(specContent), 0600)).To(Succeed())

		mockSpecGen := &mocks.SpecGenerator{}
		mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
			// Change spec to prompted
			Expect(
				os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600),
			).To(Succeed())
			// Create a non-.md file in inbox (should be skipped by approveInboxPrompts)
			Expect(
				os.WriteFile(filepath.Join(inboxDir, "notes.txt"), []byte("not a prompt"), 0600),
			).To(Succeed())
			// Also create a valid .md prompt
			promptContent := "---\ntitle: gen\nstatus: approved\n---\n# Body\n"
			return os.WriteFile(filepath.Join(inboxDir, "001-gen.md"), []byte(promptContent), 0600)
		}

		setupMocks()

		r := runner.NewOneShotRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			logsDir,
			filepath.Join(specsDir, "inbox"),
			specInProgressDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			mockSpecGen,
			libtime.NewCurrentDateTime(),
			containerChecker,
			true,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		err := r.Run(ctx)
		Expect(err).To(BeNil())

		// txt file should remain in inbox, md file should be in in-progress
		_, err = os.Stat(filepath.Join(inboxDir, "notes.txt"))
		Expect(err).To(BeNil())
		_, err = os.Stat(filepath.Join(inProgressDir, "001-gen.md"))
		Expect(err).To(BeNil())
	})

	It("should return error when ListQueued fails in loop", func() {
		setupMocks()
		manager.ListQueuedReturns(nil, context.DeadlineExceeded)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list queued prompts"))
	})

	It("should return error when ProcessQueue fails in loop", func() {
		setupMocks()
		processor.ProcessQueueReturns(context.DeadlineExceeded)

		r := newTestOneShotRunner(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("process queue"))
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
		"should return context error immediately when context is cancelled before loop runs",
		func() {
			setupMocks()
			r := newTestOneShotRunner(
				promptsDir,
				promptsDir,
				filepath.Join(promptsDir, "completed"),
			)

			cancel()

			err := r.Run(ctx)
			Expect(err).To(MatchError(context.Canceled))
			Expect(processor.ProcessQueueCallCount()).To(Equal(0))
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

	It("should handle generateFromApprovedSpecs when specsInProgressDir does not exist", func() {
		setupMocks()
		// specsInProgressDir is inside specsDir which doesn't exist at all
		nonExistentSpecDir := filepath.Join(tempDir, "no-such-specs", "in-progress")

		r := runner.NewOneShotRunner(
			promptsDir,
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "logs"),
			filepath.Join(specsDir, "inbox"),
			nonExistentSpecDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			&mocks.SpecGenerator{},
			libtime.NewCurrentDateTime(),
			containerChecker,
			false,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		// Should succeed (createDirectories creates the dir, but generateFromApprovedSpecs
		// still works gracefully with an empty dir)
		err := r.Run(ctx)
		Expect(err).To(BeNil())
		Expect(processor.ProcessQueueCallCount()).To(Equal(1))
	})

	It(
		"should continue when spec generation fails and terminate when no prompts are moved",
		func() {
			inboxDir := filepath.Join(tempDir, "inbox")
			inProgressDir := filepath.Join(tempDir, "in-progress")
			completedDir := filepath.Join(tempDir, "completed")
			logsDir := filepath.Join(tempDir, "logs")
			for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir} {
				Expect(os.MkdirAll(d, 0750)).To(Succeed())
			}

			specInProgressDir := filepath.Join(specsDir, "in-progress")
			Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

			// Spec 1 fails, spec 2 succeeds but creates no inbox files → loop exits
			spec1 := filepath.Join(specInProgressDir, "001-fail-spec.md")
			spec2 := filepath.Join(specInProgressDir, "002-ok-spec.md")
			Expect(
				os.WriteFile(spec1, []byte("---\nstatus: approved\n---\n# Fail Spec\n"), 0600),
			).To(Succeed())
			Expect(
				os.WriteFile(spec2, []byte("---\nstatus: approved\n---\n# OK Spec\n"), 0600),
			).To(Succeed())

			mockSpecGen := &mocks.SpecGenerator{}
			mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
				if filepath.Base(path) == "001-fail-spec.md" {
					return context.DeadlineExceeded
				}
				// Mark spec 2 as prompted — but don't create any inbox files
				return os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# OK Spec\n"), 0600)
			}

			setupMocks()

			r := runner.NewOneShotRunner(
				inboxDir,
				inProgressDir,
				completedDir,
				logsDir,
				filepath.Join(specsDir, "inbox"),
				specInProgressDir,
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				processor,
				mockSpecGen,
				libtime.NewCurrentDateTime(),
				containerChecker,
				false,
				&mocks.SpecSlugMigrator{},
				&mocks.FileMover{},
				nil,
			)

			err := r.Run(ctx)
			Expect(err).To(BeNil())
			// Both specs were attempted in first iteration; 0 inbox prompts moved → loop exits
			Expect(mockSpecGen.GenerateCallCount()).To(Equal(2))
			// ProcessQueue called once (generated=0 after no prompts moved, queued=0 → break)
			Expect(processor.ProcessQueueCallCount()).To(Equal(1))
		},
	)

	It("should return error when NormalizeFilenames fails in approveInboxPrompts", func() {
		inboxDir := filepath.Join(tempDir, "inbox")
		inProgressDir := filepath.Join(tempDir, "in-progress")
		completedDir := filepath.Join(tempDir, "completed")
		logsDir := filepath.Join(tempDir, "logs")
		for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir} {
			Expect(os.MkdirAll(d, 0750)).To(Succeed())
		}

		specInProgressDir := filepath.Join(specsDir, "in-progress")
		Expect(os.MkdirAll(specInProgressDir, 0750)).To(Succeed())

		specContent := "---\nstatus: approved\n---\n# My Spec\n"
		specFile := filepath.Join(specInProgressDir, "001-my-spec.md")
		Expect(os.WriteFile(specFile, []byte(specContent), 0600)).To(Succeed())

		mockSpecGen := &mocks.SpecGenerator{}
		mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
			return os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600)
		}

		locker.AcquireReturns(nil)
		locker.ReleaseReturns(nil)
		processor.ProcessQueueReturns(nil)

		// NormalizeFilenames fails (used both in initial normalize and in approveInboxPrompts)
		callCount := 0
		manager.NormalizeFilenamesStub = func(manCtx context.Context, dir string) ([]prompt.Rename, error) {
			callCount++
			if callCount >= 2 {
				// Fail on second call (inside approveInboxPrompts)
				return nil, context.DeadlineExceeded
			}
			return nil, nil
		}

		r := runner.NewOneShotRunner(
			inboxDir,
			inProgressDir,
			completedDir,
			logsDir,
			filepath.Join(specsDir, "inbox"),
			specInProgressDir,
			filepath.Join(specsDir, "completed"),
			filepath.Join(specsDir, "logs"),
			manager,
			locker,
			processor,
			mockSpecGen,
			libtime.NewCurrentDateTime(),
			containerChecker,
			true,
			&mocks.SpecSlugMigrator{},
			&mocks.FileMover{},
			nil,
		)

		err := r.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("generate from approved specs"))
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

	Describe("autoApprove flag", func() {
		buildSpecDirs := func() (string, string) {
			inboxDir := filepath.Join(tempDir, "inbox")
			inProgressDir := filepath.Join(tempDir, "in-progress")
			completedDir := filepath.Join(tempDir, "completed")
			logsDir := filepath.Join(tempDir, "logs")
			specInProgressDir := filepath.Join(specsDir, "in-progress")
			for _, d := range []string{inboxDir, inProgressDir, completedDir, logsDir, specInProgressDir} {
				Expect(os.MkdirAll(d, 0750)).To(Succeed())
			}
			specContent := "---\nstatus: approved\n---\n# My Spec\n"
			specFile := filepath.Join(specInProgressDir, "001-my-spec.md")
			Expect(os.WriteFile(specFile, []byte(specContent), 0600)).To(Succeed())
			return inboxDir, specInProgressDir
		}

		buildMockGen := func(inboxDir string, specInProgressDir string) *mocks.SpecGenerator {
			mockSpecGen := &mocks.SpecGenerator{}
			mockSpecGen.GenerateStub = func(genCtx context.Context, path string) error {
				Expect(
					os.WriteFile(path, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600),
				).To(Succeed())
				promptContent := "---\ntitle: generated prompt\nstatus: approved\n---\n# Body\n"
				return os.WriteFile(
					filepath.Join(inboxDir, "001-gen-prompt.md"),
					[]byte(promptContent),
					0600,
				)
			}
			return mockSpecGen
		}

		It("autoApprove=true: generated prompts are moved to in-progress and executed", func() {
			inboxDir, specInProgressDir := buildSpecDirs()
			inProgressDir := filepath.Join(tempDir, "in-progress")
			completedDir := filepath.Join(tempDir, "completed")
			logsDir := filepath.Join(tempDir, "logs")

			mockSpecGen := buildMockGen(inboxDir, specInProgressDir)
			setupMocks()

			r := runner.NewOneShotRunner(
				inboxDir,
				inProgressDir,
				completedDir,
				logsDir,
				filepath.Join(specsDir, "inbox"),
				specInProgressDir,
				filepath.Join(specsDir, "completed"),
				filepath.Join(specsDir, "logs"),
				manager,
				locker,
				processor,
				mockSpecGen,
				libtime.NewCurrentDateTime(),
				containerChecker,
				true,
				&mocks.SpecSlugMigrator{},
				&mocks.FileMover{},
				nil,
			)

			err := r.Run(ctx)
			Expect(err).To(BeNil())
			Expect(mockSpecGen.GenerateCallCount()).To(Equal(1))

			// Prompt must be moved from inbox to in-progress
			_, statErr := os.Stat(filepath.Join(inProgressDir, "001-gen-prompt.md"))
			Expect(statErr).To(BeNil())
			_, statErr = os.Stat(filepath.Join(inboxDir, "001-gen-prompt.md"))
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It(
			"autoApprove=false: generated prompts remain in inbox, only pre-queued prompts execute",
			func() {
				inboxDir, specInProgressDir := buildSpecDirs()
				inProgressDir := filepath.Join(tempDir, "in-progress")
				completedDir := filepath.Join(tempDir, "completed")
				logsDir := filepath.Join(tempDir, "logs")

				mockSpecGen := buildMockGen(inboxDir, specInProgressDir)
				setupMocks()

				r := runner.NewOneShotRunner(
					inboxDir,
					inProgressDir,
					completedDir,
					logsDir,
					filepath.Join(specsDir, "inbox"),
					specInProgressDir,
					filepath.Join(specsDir, "completed"),
					filepath.Join(specsDir, "logs"),
					manager,
					locker,
					processor,
					mockSpecGen,
					libtime.NewCurrentDateTime(),
					containerChecker,
					false,
					&mocks.SpecSlugMigrator{},
					&mocks.FileMover{},
					nil,
				)

				err := r.Run(ctx)
				Expect(err).To(BeNil())
				Expect(mockSpecGen.GenerateCallCount()).To(Equal(1))

				// Prompt must remain in inbox (not moved to in-progress)
				_, statErr := os.Stat(filepath.Join(inboxDir, "001-gen-prompt.md"))
				Expect(statErr).To(BeNil())
				_, statErr = os.Stat(filepath.Join(inProgressDir, "001-gen-prompt.md"))
				Expect(os.IsNotExist(statErr)).To(BeTrue())

				// ProcessQueue still called (pre-queued prompts execute)
				Expect(processor.ProcessQueueCallCount()).To(BeNumerically(">=", 1))
			},
		)
	})
})
