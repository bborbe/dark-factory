// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Processor", func() {
	var (
		tempDir        string
		promptsDir     string
		ready          chan struct{}
		ctx            context.Context
		cancel         context.CancelFunc
		mockExecutor   *mocks.Executor
		mockManager    *mocks.Manager
		mockReleaser   *mocks.Releaser
		mockVersionGet *mocks.VersionGetter
		mockBrancher   *mocks.Brancher
		mockPRCreator  *mocks.PRCreator
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "processor-test-*")
		Expect(err).NotTo(HaveOccurred())

		promptsDir = filepath.Join(tempDir, "prompts")
		err = os.MkdirAll(promptsDir, 0750)
		Expect(err).NotTo(HaveOccurred())

		ready = make(chan struct{}, 10)
		ctx, cancel = context.WithCancel(context.Background())

		mockExecutor = &mocks.Executor{}
		mockManager = &mocks.Manager{}
		mockReleaser = &mocks.Releaser{}
		mockVersionGet = &mocks.VersionGetter{}
		mockBrancher = &mocks.Brancher{}
		mockPRCreator = &mocks.PRCreator{}
		mockVersionGet.GetReturns("v0.0.1-test")
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("should start and stop cleanly", func() {
		mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Let it run briefly
		time.Sleep(200 * time.Millisecond)

		// Cancel and verify clean shutdown
		cancel()

		select {
		case err := <-errCh:
			Expect(err).To(BeNil())
		case <-time.After(2 * time.Second):
			Fail("processor did not stop within timeout")
		}
	})

	It("should process existing queued prompt on startup", func() {
		promptPath := filepath.Join(promptsDir, "001-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		// First call returns prompt, second call returns empty (processed)
		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Test prompt", nil)
		mockManager.TitleReturns("Test prompt", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockExecutor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was called with correct log path
		_, _, logFile, containerName := mockExecutor.ExecuteArgsForCall(0)
		Expect(logFile).To(Equal(filepath.Join(promptsDir, "log", "001-test.log")))
		Expect(containerName).To(Equal("dark-factory-001-test"))

		// Verify status was set to executing
		Expect(mockManager.SetStatusCallCount()).To(BeNumerically(">=", 1))

		// Verify moved to completed
		Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

		cancel()
	})

	It("should process prompts when ready signal received", func() {
		promptPath := filepath.Join(promptsDir, "001-signal.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		// Initially no prompts
		mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Signal test", nil)
		mockManager.TitleReturns("Signal test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for initial scan
		time.Sleep(200 * time.Millisecond)

		// Now return a prompt and send ready signal
		mockManager.ListQueuedReturnsOnCall(1, queued, nil)
		mockManager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)
		ready <- struct{}{}

		// Wait for processing
		Eventually(func() int {
			return mockExecutor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		cancel()
	})

	It("should skip empty prompts", func() {
		promptPath := filepath.Join(promptsDir, "001-empty.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("", prompt.ErrEmptyPrompt)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockManager.MoveToCompletedCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was NOT called
		Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should handle executor errors and mark prompt as failed", func() {
		promptPath := filepath.Join(promptsDir, "001-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturns(queued, nil)
		mockManager.ContentReturns("# Fail test", nil)
		mockManager.TitleReturns("Fail test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(stderrors.New("execution failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Wait for error
		select {
		case err := <-errCh:
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("execution failed"))
		case <-time.After(2 * time.Second):
			Fail("processor did not return error within timeout")
		}

		// Verify status was set to failed
		Expect(mockManager.SetStatusCallCount()).To(BeNumerically(">=", 2))
		// Last call should be setting to "failed"
		lastCall := mockManager.SetStatusCallCount() - 1
		_, _, status := mockManager.SetStatusArgsForCall(lastCall)
		Expect(status).To(Equal("failed"))

		cancel()
	})

	It("should call CommitOnly when no changelog", func() {
		promptPath := filepath.Join(promptsDir, "001-no-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# No changelog test", nil)
		mockManager.TitleReturns("No changelog test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockReleaser.CommitOnlyCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify CommitAndRelease was NOT called
		Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with PatchBump when changelog exists", func() {
		promptPath := filepath.Join(promptsDir, "001-with-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Fix bug", nil)
		mockManager.TitleReturns("Fix bug", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(true)
		mockReleaser.GetNextVersionReturns("v0.1.1", nil)
		mockReleaser.CommitAndReleaseReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockReleaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify PatchBump was used
		_, _, bump := mockReleaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.PatchBump))

		// Verify CommitOnly was NOT called
		Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with MinorBump for feature title", func() {
		promptPath := filepath.Join(promptsDir, "001-feature.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Add new feature", nil)
		mockManager.TitleReturns("Add new feature", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(true)
		mockReleaser.GetNextVersionReturns("v0.2.0", nil)
		mockReleaser.CommitAndReleaseReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockReleaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify MinorBump was used
		_, _, bump := mockReleaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.MinorBump))

		cancel()
	})

	It("should process multiple queued prompts sequentially", func() {
		promptPath1 := filepath.Join(promptsDir, "001-first.md")
		promptPath2 := filepath.Join(promptsDir, "002-second.md")

		// Return both prompts first, then just second, then none
		mockManager.ListQueuedReturnsOnCall(0, []prompt.Prompt{
			{Path: promptPath1, Status: prompt.StatusQueued},
			{Path: promptPath2, Status: prompt.StatusQueued},
		}, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{
			{Path: promptPath2, Status: prompt.StatusQueued},
		}, nil)
		mockManager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)

		mockManager.ContentReturns("# Test", nil)
		mockManager.TitleReturns("Test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for both to be processed
		Eventually(func() int {
			return mockExecutor.ExecuteCallCount()
		}, 3*time.Second, 50*time.Millisecond).Should(Equal(2))

		cancel()
	})

	It("should sanitize container name", func() {
		promptPath := filepath.Join(promptsDir, "001-test@file#name.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Test", nil)
		mockManager.TitleReturns("Test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockExecutor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify container name was sanitized
		_, _, _, containerName := mockExecutor.ExecuteArgsForCall(0)
		Expect(containerName).To(Equal("dark-factory-001-test-file-name"))

		cancel()
	})

	It("should skip prompt with invalid status", func() {
		promptPath := filepath.Join(promptsDir, "001-invalid-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusExecuting}, // Wrong status for execution
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should skip prompt when previous not completed", func() {
		promptPath := filepath.Join(promptsDir, "003-third.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.AllPreviousCompletedReturns(false) // Previous prompts not completed

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should set version in frontmatter from version getter", func() {
		promptPath := filepath.Join(promptsDir, "001-version-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.ContentReturns("# Version test", nil)
		mockManager.TitleReturns("Version test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
		mockManager.SetStatusReturns(nil)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			config.WorkflowDirect,
			mockBrancher,
			mockPRCreator,
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return mockManager.SetVersionCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify version was set with correct value
		_, path, version := mockManager.SetVersionArgsForCall(0)
		Expect(path).To(Equal(promptPath))
		Expect(version).To(Equal("v0.0.1-test"))

		cancel()
	})

	Describe("PR Workflow", func() {
		It("should create branch, commit, push, create PR, and switch back", func() {
			promptPath := filepath.Join(promptsDir, "001-pr-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.ContentReturns("# PR test", nil)
			mockManager.TitleReturns("Add new feature", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.CurrentBranchReturns("master", nil)
			mockBrancher.CreateAndSwitchReturns(nil)
			mockBrancher.PushReturns(nil)
			mockBrancher.SwitchReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/123", nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				config.WorkflowPR,
				mockBrancher,
				mockPRCreator,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify brancher calls
			Expect(mockBrancher.CurrentBranchCallCount()).To(Equal(1))
			Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(1))
			_, branchName := mockBrancher.CreateAndSwitchArgsForCall(0)
			Expect(branchName).To(Equal("dark-factory/001-pr-test"))

			Expect(mockBrancher.PushCallCount()).To(Equal(1))
			_, pushedBranch := mockBrancher.PushArgsForCall(0)
			Expect(pushedBranch).To(Equal("dark-factory/001-pr-test"))

			Expect(mockBrancher.SwitchCallCount()).To(Equal(1))
			_, switchedBranch := mockBrancher.SwitchArgsForCall(0)
			Expect(switchedBranch).To(Equal("master"))

			// Verify PR was created
			_, title, body := mockPRCreator.CreateArgsForCall(0)
			Expect(title).To(Equal("Add new feature"))
			Expect(body).To(Equal("Automated by dark-factory"))

			// Verify CommitOnly was called, not CommitAndRelease
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))
			Expect(mockReleaser.HasChangelogCallCount()).To(Equal(0))

			cancel()
		})

		It("should handle branch creation error", func() {
			promptPath := filepath.Join(promptsDir, "001-branch-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}

			mockManager.ListQueuedReturns(queued, nil)
			mockManager.ContentReturns("# Branch error test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockBrancher.CurrentBranchReturns("master", nil)
			mockBrancher.CreateAndSwitchReturns(stderrors.New("branch already exists"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				config.WorkflowPR,
				mockBrancher,
				mockPRCreator,
			)

			// Run processor in goroutine
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for error
			select {
			case err := <-errCh:
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("branch already exists"))
			case <-time.After(2 * time.Second):
				Fail("processor did not return error within timeout")
			}

			// Verify status was set to failed
			Expect(mockManager.SetStatusCallCount()).To(BeNumerically(">=", 2))
			lastCall := mockManager.SetStatusCallCount() - 1
			_, _, status := mockManager.SetStatusArgsForCall(lastCall)
			Expect(status).To(Equal("failed"))

			// Verify executor was NOT called
			Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

			cancel()
		})

		It("should handle PR creation error after successful push", func() {
			promptPath := filepath.Join(promptsDir, "001-pr-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}

			mockManager.ListQueuedReturns(queued, nil)
			mockManager.ContentReturns("# PR error test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.CurrentBranchReturns("master", nil)
			mockBrancher.CreateAndSwitchReturns(nil)
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("", stderrors.New("gh pr create failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				config.WorkflowPR,
				mockBrancher,
				mockPRCreator,
			)

			// Run processor in goroutine
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for error
			select {
			case err := <-errCh:
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("gh pr create failed"))
			case <-time.After(2 * time.Second):
				Fail("processor did not return error within timeout")
			}

			// Verify branch operations succeeded
			Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(1))
			Expect(mockBrancher.PushCallCount()).To(Equal(1))

			// Verify status was set to failed
			Expect(mockManager.SetStatusCallCount()).To(BeNumerically(">=", 2))
			lastCall := mockManager.SetStatusCallCount() - 1
			_, _, status := mockManager.SetStatusArgsForCall(lastCall)
			Expect(status).To(Equal("failed"))

			cancel()
		})
	})
})
