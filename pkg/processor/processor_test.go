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
	"github.com/bborbe/dark-factory/pkg/report"
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

	// Helper function to create a PromptFile mock
	createMockPromptFile := func(path string, body string) *prompt.PromptFile {
		return &prompt.PromptFile{
			Path: path,
			Body: []byte(body),
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.StatusQueued),
			},
		}
	}

	// Set up default Load behavior to return a valid PromptFile
	BeforeEach(func() {
		mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
			return createMockPromptFile(path, "# Test\n\nDefault test content"), nil
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

		// Verify Load was called (processor uses Load/Save pattern now)
		Expect(mockManager.LoadCallCount()).To(BeNumerically(">=", 1))

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
		// Override Load to return empty body
		mockManager.LoadReturns(&prompt.PromptFile{
			Path: promptPath,
			Body: []byte(""),
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.StatusQueued),
			},
		}, nil)
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

		// Return queued once, then empty (so loop exits after failure)
		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
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

		// Run processor — marks failed and continues (no error returned)
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for Load to be called (marks prompt as failed)
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

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
		// Override Load to return PromptFile with title "Add new feature"
		mockManager.LoadReturns(&prompt.PromptFile{
			Path: promptPath,
			Body: []byte("# Add new feature\n\nImplement new feature."),
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.StatusQueued),
			},
		}, nil)
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

		// Wait for processing (processor uses Load/PrepareForExecution now)
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify Load was called with correct path
		_, path := mockManager.LoadArgsForCall(0)
		Expect(path).To(Equal(promptPath))

		cancel()
	})

	It("should append completion report suffix to content before executor call", func() {
		promptPath := filepath.Join(promptsDir, "001-suffix-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.StatusQueued},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with specific content
		mockManager.LoadReturns(&prompt.PromptFile{
			Path: promptPath,
			Body: []byte("# Test prompt content\n\nContent for testing suffix."),
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.StatusQueued),
			},
		}, nil)
		mockManager.ContentReturns("# Test prompt content", nil)
		mockManager.TitleReturns("Suffix test", nil)
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

		// Verify executor was called with content including suffix
		_, promptContent, _, _ := mockExecutor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring("# Test prompt content"))
		Expect(promptContent).To(ContainSubstring("DARK-FACTORY-REPORT"))
		Expect(promptContent).To(ContainSubstring("Completion Report (MANDATORY)"))
		Expect(promptContent).To(HaveSuffix(report.Suffix()))

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
			// Override Load to return PromptFile with title "Add new feature"
			mockManager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Body: []byte("# Add new feature\n\nPR test content."),
				Frontmatter: prompt.Frontmatter{
					Status: string(prompt.StatusQueued),
				},
			}, nil)
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

			// Return queued once, then empty
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
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

			// Run processor — marks failed and continues
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for Load to be called (marks prompt as failed)
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify executor was NOT called
			Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

			cancel()
		})

		It("should handle PR creation error after successful push", func() {
			promptPath := filepath.Join(promptsDir, "001-pr-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}

			// Return queued once, then empty
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
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

			// Run processor — marks failed and continues
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for Load to be called (marks prompt as failed)
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify branch operations succeeded
			Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(1))
			Expect(mockBrancher.PushCallCount()).To(Equal(1))

			cancel()
		})
	})

	Describe("Completion Report Parsing", func() {
		It("should continue to commit when report status is success", func() {
			promptPath := filepath.Join(promptsDir, "001-report-success.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.ContentReturns("# Report success test", nil)
			mockManager.TitleReturns("Report success test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Completed successfully","blockers":[]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
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

			// Verify moved to completed
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})

		It("should mark failed and continue when report status is failed", func() {
			promptPath := filepath.Join(promptsDir, "001-report-failed.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.ContentReturns("# Report failed test", nil)
			mockManager.TitleReturns("Report failed test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with failed report
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"failed","summary":"Could not complete","blockers":["tests failed","lint errors"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				config.WorkflowDirect,
				mockBrancher,
				mockPRCreator,
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should mark failed and continue when report status is partial", func() {
			promptPath := filepath.Join(promptsDir, "001-report-partial.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.ContentReturns("# Report partial test", nil)
			mockManager.TitleReturns("Report partial test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with partial report
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"partial","summary":"Half done","blockers":["make precommit fails"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				config.WorkflowDirect,
				mockBrancher,
				mockPRCreator,
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should continue when no report found (backwards compatible)", func() {
			promptPath := filepath.Join(promptsDir, "001-no-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.ContentReturns("# No report test", nil)
			mockManager.TitleReturns("No report test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			// Mock executor writes log WITHOUT report (old-style prompt)
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output
more output
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
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

			// Verify moved to completed
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})

		It("should continue when report parsing fails (graceful degradation)", func() {
			promptPath := filepath.Join(promptsDir, "001-malformed-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.StatusQueued},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.ContentReturns("# Malformed report test", nil)
			mockManager.TitleReturns("Malformed report test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			// Mock executor writes log with malformed JSON
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{this is not valid json}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
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

			// Wait for processing - should continue despite malformed JSON
			Eventually(func() int {
				return mockReleaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})
	})
})
