// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("Processor", func() {
	var (
		tempDir           string
		promptsDir        string
		ready             chan struct{}
		ctx               context.Context
		cancel            context.CancelFunc
		mockExecutor      *mocks.Executor
		mockManager       *mocks.Manager
		mockReleaser      *mocks.Releaser
		mockVersionGet    *mocks.VersionGetter
		mockBrancher      *mocks.Brancher
		mockPRCreator     *mocks.PRCreator
		mockCloner        *mocks.Cloner
		mockPRMerger      *mocks.PRMerger
		mockAutoCompleter *mocks.AutoCompleter
		mockSpecLister    *mocks.Lister
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
		mockCloner = &mocks.Cloner{}
		mockPRMerger = &mocks.PRMerger{}
		mockAutoCompleter = &mocks.AutoCompleter{}
		mockSpecLister = &mocks.Lister{}
		mockSpecLister.ListReturns(nil, nil)
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
		return prompt.NewPromptFile(
			path,
			prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
			[]byte(body),
			libtime.NewCurrentDateTime(),
		)
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
		Expect(containerName).To(Equal("test-project-001-test"))

		// Verify Load was called (processor uses Load/Save pattern now)
		Expect(mockManager.LoadCallCount()).To(BeNumerically(">=", 1))

		// Verify moved to completed
		Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

		cancel()
	})

	It("should process prompts when ready signal received", func() {
		promptPath := filepath.Join(promptsDir, "001-signal.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return empty body
		mockManager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte(""),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		mockManager.ContentReturns("", prompt.ErrEmptyPrompt)
		mockManager.MoveToCompletedReturns(nil)
		mockManager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
		_, bump := mockReleaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.PatchBump))

		// Verify CommitOnly was NOT called
		Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with MinorBump for feature title", func() {
		// Create CHANGELOG.md with "Add new feature" in Unreleased section
		// determineBump reads from current directory
		originalDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		err = os.Chdir(tempDir)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = os.Chdir(originalDir)
		}()

		err = os.WriteFile(
			"CHANGELOG.md",
			[]byte("## Unreleased\n\n- feat: Add new feature\n"),
			0600,
		)
		Expect(err).NotTo(HaveOccurred())

		promptPath := filepath.Join(promptsDir, "001-feature.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with title "Add new feature"
		mockManager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Add new feature\n\nImplement new feature."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
		_, bump := mockReleaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.MinorBump))

		cancel()
	})

	It("should process multiple queued prompts sequentially", func() {
		promptPath1 := filepath.Join(promptsDir, "001-first.md")
		promptPath2 := filepath.Join(promptsDir, "002-second.md")

		// Return both prompts first, then just second, then none
		mockManager.ListQueuedReturnsOnCall(0, []prompt.Prompt{
			{Path: promptPath1, Status: prompt.ApprovedPromptStatus},
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
		}, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
		Expect(containerName).To(Equal("test-project-001-test-file-name"))

		cancel()
	})

	It("should auto-set status to queued when prompt has non-standard status", func() {
		promptPath := filepath.Join(promptsDir, "001-auto-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: ""}, // empty status triggers auto-set
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.SetStatusReturns(nil)
		mockManager.ContentReturns("# Auto status test", nil)
		mockManager.TitleReturns("Auto status test", nil)
		mockManager.SetContainerReturns(nil)
		mockManager.SetVersionReturns(nil)
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for SetStatus to be called (auto-set to queued)
		Eventually(func() int {
			return mockManager.SetStatusCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify SetStatus was called with "approved"
		_, _, status := mockManager.SetStatusArgsForCall(0)
		Expect(status).To(Equal("approved"))

		cancel()
	})

	It("should skip prompt with invalid status", func() {
		promptPath := filepath.Join(promptsDir, "001-invalid-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ExecutingPromptStatus}, // Wrong status for execution
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.AllPreviousCompletedReturns(false) // Previous prompts not completed

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with specific content
		mockManager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test prompt content\n\nContent for testing suffix."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
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

	It("should append validation suffix when validationCommand is set", func() {
		promptPath := filepath.Join(promptsDir, "001-validation-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		mockManager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Validation test\n\nContent for validation suffix test."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
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
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"make precommit",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return mockExecutor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := mockExecutor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring(report.ValidationSuffix("make precommit")))

		cancel()
	})

	Describe("Worktree Workflow", func() {
		It("should add worktree, commit, push, create PR, and remove worktree", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Add new feature\n\nWorktree test content."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Worktree test", nil)
			mockManager.TitleReturns("Add new feature", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			// Mock worktree.Add to create the actual directory
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/123", nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree operations
			Expect(mockCloner.CloneCallCount()).To(Equal(1))
			_, _, worktreePath, branchName := mockCloner.CloneArgsForCall(0)
			Expect(worktreePath).To(ContainSubstring("dark-factory/test-project-001-worktree-test"))
			Expect(branchName).To(Equal("dark-factory/001-worktree-test"))

			// Verify push was called
			Expect(mockBrancher.PushCallCount()).To(Equal(1))
			_, pushedBranch := mockBrancher.PushArgsForCall(0)
			Expect(pushedBranch).To(Equal("dark-factory/001-worktree-test"))

			// Verify PR was created
			_, title, body := mockPRCreator.CreateArgsForCall(0)
			Expect(title).To(Equal("Add new feature"))
			Expect(body).To(Equal("Automated by dark-factory"))

			// Verify worktree was removed
			Expect(mockCloner.RemoveCallCount()).To(BeNumerically(">=", 1))
			_, removedPath := mockCloner.RemoveArgsForCall(0)
			Expect(removedPath).To(ContainSubstring("dark-factory/test-project-001-worktree-test"))

			// Verify CommitOnly was called, not CommitAndRelease
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))
			// HasChangelog is called once for changelog suffix check during content assembly
			Expect(mockReleaser.HasChangelogCallCount()).To(Equal(1))

			cancel()
		})

		It("should pass absolute log file path in original directory to executor", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-logpath-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err = os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Log path test\n\nVerify log path is absolute."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Log path test", nil)
			mockManager.TitleReturns("Log path test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/repo/pull/99", nil)

			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				return nil
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return mockExecutor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Read captured args after Eventually confirms execution happened (safe - goroutine done writing)
			_, _, capturedLogFile, _ := mockExecutor.ExecuteArgsForCall(0)

			// Log file must be absolute and point to the original log dir, not the clone dir
			Expect(filepath.IsAbs(capturedLogFile)).To(BeTrue(), "log file path should be absolute")
			Expect(capturedLogFile).To(Equal(filepath.Join(logDir, "001-logpath-test.log")))
			Expect(capturedLogFile).NotTo(ContainSubstring("dark-factory/test-project-"))

			cancel()
		})

		It("should processes prompt, create PR and auto-merge via worktree", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-worktree-automerge.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Add feature\n\nWorktree auto-merge test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Worktree automerge test", nil)
			mockManager.TitleReturns("Add feature", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/repo/pull/2", nil)
			mockPRMerger.WaitAndMergeReturns(nil)
			mockBrancher.DefaultBranchReturns("master", nil)
			mockBrancher.SwitchReturns(nil)
			mockBrancher.PullReturns(nil)
			mockReleaser.HasChangelogReturns(false)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				true, // autoMerge enabled
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to be called
			Eventually(func() int {
				return mockPRMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree operations
			Expect(mockCloner.CloneCallCount()).To(Equal(1))
			Expect(mockPRCreator.CreateCallCount()).To(Equal(1))
			Expect(mockCloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Verify WaitAndMerge was called with correct PR URL
			_, mergedURL := mockPRMerger.WaitAndMergeArgsForCall(0)
			Expect(mergedURL).To(Equal("https://github.com/test/repo/pull/2"))

			// Verify post-merge actions
			Eventually(func() int {
				return mockBrancher.DefaultBranchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))
			Eventually(func() int {
				return mockBrancher.PullCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})

		It("should clean up worktree even on execution failure", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nFail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Fail test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			// Mock worktree.Add to create the actual directory
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockExecutor.ExecuteReturns(stderrors.New("execution failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for worktree to be cleaned up (via defer)
			Eventually(func() int {
				return mockCloner.RemoveCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify worktree was added
			Expect(mockCloner.CloneCallCount()).To(Equal(1))

			// Verify worktree was removed despite failure
			Expect(mockCloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			cancel()
		})

		It("should log warning but not fail when worktree removal fails", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-remove-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nRemove fail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			// Mock worktree.Add to create the actual directory
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			// Mock worktree.Remove to return an error but don't actually remove
			mockCloner.RemoveReturns(stderrors.New("worktree removal failed"))
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/123", nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing - should complete despite remove failure
			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree removal was attempted
			Expect(mockCloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Verify PR was created successfully despite cleanup failure
			Expect(mockPRCreator.CreateCallCount()).To(Equal(1))

			cancel()
		})

		It("should mark prompt failed when WaitAndMerge fails in worktree workflow", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-worktree-merge-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nWorktree merge fail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.ContentReturns("# Test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/repo/pull/3", nil)
			mockPRMerger.WaitAndMergeReturns(stderrors.New("PR has conflicts"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				true, // autoMerge enabled
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to be called
			Eventually(func() int {
				return mockPRMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Worktree was added and removed before merge attempt
			Expect(mockCloner.CloneCallCount()).To(Equal(1))
			Expect(mockCloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Load should be called to mark prompt as failed
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})

		It("should handle worktree add error", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-add-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.ContentReturns("# Worktree add error test", nil)
			mockManager.TitleReturns("Test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockCloner.CloneReturns(stderrors.New("clone failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor
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
	})

	Describe("Completion Report Parsing", func() {
		It("should store summary in frontmatter when report has summary", func() {
			promptPath := filepath.Join(promptsDir, "001-summary-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			completedDir := filepath.Join(promptsDir, "completed")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())
			err = os.MkdirAll(completedDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Track the PromptFile that gets loaded/saved
			var savedPromptFile *prompt.PromptFile

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				// Create a real PromptFile with a temporary backing file
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Summary test\n\nTest content"),
					libtime.NewCurrentDateTime(),
				)
				savedPromptFile = pf
				return pf, nil
			}
			mockManager.ContentReturns("# Summary test", nil)
			mockManager.TitleReturns("Summary test", nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report containing summary
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Added feature successfully","blockers":[]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			// Mock MoveToCompleted to verify frontmatter has summary
			mockManager.MoveToCompletedStub = func(ctx context.Context, path string) error {
				// Verify summary was set before moving
				Expect(savedPromptFile.Frontmatter.Summary).To(Equal("Added feature successfully"))
				return nil
			}

			p := processor.NewProcessor(
				promptsDir,
				completedDir,
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return mockManager.MoveToCompletedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify summary was stored
			Expect(savedPromptFile.Frontmatter.Summary).To(Equal("Added feature successfully"))

			cancel()
		})

		It("should continue to commit when report status is success", func() {
			promptPath := filepath.Join(promptsDir, "001-report-success.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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

		It("should override success to partial when verification exitCode is non-zero", func() {
			promptPath := filepath.Join(promptsDir, "001-verification-override.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			// Return queued once, then empty (so loop exits)
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.ContentReturns("# Verification override test", nil)
			mockManager.TitleReturns("Verification override test", nil)
			mockManager.SetContainerReturns(nil)
			mockManager.SetVersionReturns(nil)
			mockManager.SetStatusReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with success report but non-zero verification exit code
			mockExecutor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Task completed","blockers":[],"verification":{"command":"make precommit","exitCode":1}}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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

			// Verify moved to completed was NOT called (status was overridden to partial)
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should continue when report parsing fails (graceful degradation)", func() {
			promptPath := filepath.Join(promptsDir, "001-malformed-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
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
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
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

	Describe("Git sync before execution", func() {
		It("should call Fetch and MergeOriginDefault before processing prompt", func() {
			promptPath := filepath.Join(promptsDir, "001-sync-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return mockExecutor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify Fetch and MergeOriginDefault were called
			Expect(mockBrancher.FetchCallCount()).To(Equal(1))
			Expect(mockBrancher.MergeOriginDefaultCallCount()).To(Equal(1))

			cancel()
		})

		It("should fail prompt if Fetch fails", func() {
			promptPath := filepath.Join(promptsDir, "001-fetch-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockBrancher.FetchReturns(stderrors.New("fetch failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for Fetch to be called
			Eventually(func() int {
				return mockBrancher.FetchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Executor should NOT be called since Fetch failed
			Consistently(func() int {
				return mockExecutor.ExecuteCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Load should be called to mark as failed
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})

		It("should fail prompt if MergeOriginDefault fails", func() {
			promptPath := filepath.Join(promptsDir, "001-merge-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(stderrors.New("merge conflict"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for MergeOriginDefault to be called
			Eventually(func() int {
				return mockBrancher.MergeOriginDefaultCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Executor should NOT be called since MergeOriginDefault failed
			Consistently(func() int {
				return mockExecutor.ExecuteCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Load should be called to mark as failed
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})
	})

	Describe("PR workflow error paths", func() {
		It("should stop before push when CommitOnly fails", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-commit-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Commit error test\n\nContent."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(stderrors.New("commit failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for failure to be handled (Load called to mark as failed)
			Eventually(func() int {
				return mockManager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Push and PR creation must NOT be called
			Expect(mockBrancher.PushCallCount()).To(Equal(0))
			Expect(mockPRCreator.CreateCallCount()).To(Equal(0))

			cancel()
		})

		It("should stop before PR creation when push fails", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-push-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, nil, nil)
			mockManager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Push error test\n\nContent."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.PushReturns(stderrors.New("push failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for push to be attempted
			Eventually(func() int {
				return mockBrancher.PushCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// PR creation must NOT be called
			Expect(mockPRCreator.CreateCallCount()).To(Equal(0))

			cancel()
		})
	})

	Describe("shouldSkipPrompt", func() {
		It("should silently skip previously-failed prompt when file is unchanged", func() {
			// Create a real file so os.Stat works and modtime is captured
			promptPath := filepath.Join(promptsDir, "001-skip-unchanged.md")
			err := os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ExecutingPromptStatus},
			}

			// Call 0: first skip — validation fails → recorded in skippedPrompts
			// Call 1: second skip — same modtime → silently skipped
			// Call 2: empty → exit loop
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, queued, nil)
			mockManager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for at least 3 ListQueued calls (two skips + empty)
			Eventually(func() int {
				return mockManager.ListQueuedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 3))

			// Executor was never called — prompt was skipped both times
			Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

			cancel()
		})

		It("should retry previously-skipped prompt after file is modified", func() {
			// Create a real file so os.Stat works
			promptPath := filepath.Join(promptsDir, "001-retry-modified.md")
			err := os.WriteFile(promptPath, []byte("---\nstatus: executing\n---\n"), 0600)
			Expect(err).NotTo(HaveOccurred())

			firstQueued := []prompt.Prompt{{Path: promptPath, Status: prompt.ExecutingPromptStatus}}
			secondQueued := []prompt.Prompt{{Path: promptPath, Status: prompt.ApprovedPromptStatus}}

			callCount := 0
			mockManager.ListQueuedStub = func(_ context.Context) ([]prompt.Prompt, error) {
				callCount++
				switch callCount {
				case 1:
					// First call: StatusExecuting → fails validation → recorded with T1
					return firstQueued, nil
				case 2:
					// Modify file to change modtime → T2 (different from T1)
					time.Sleep(time.Millisecond)
					_ = os.WriteFile(
						promptPath,
						[]byte("---\nstatus: queued\n---\n\n# Test\n\nContent."),
						0600,
					)
					// Return ApprovedPromptStatus → shouldSkipPrompt: T2 != T1 → re-validate → passes
					return secondQueued, nil
				default:
					return []prompt.Prompt{}, nil
				}
			}
			mockManager.AllPreviousCompletedReturns(true)
			// Fail fast after shouldSkipPrompt passes to keep test simple
			mockBrancher.FetchReturns(stderrors.New("fetch failed after retry"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Fetch is called only when shouldSkipPrompt returns false (i.e., prompt was retried)
			Eventually(func() int {
				return mockBrancher.FetchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})
	})

	It("should return error when CommitAndRelease fails in direct workflow", func() {
		promptPath := filepath.Join(promptsDir, "001-commitrelease-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(true)
		mockReleaser.GetNextVersionReturns("v0.1.1", nil)
		mockReleaser.CommitAndReleaseReturns(stderrors.New("commit and release failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for CommitAndRelease to be called
		Eventually(func() int {
			return mockReleaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load should be called to mark prompt as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when MoveToCompleted fails for empty prompt", func() {
		promptPath := filepath.Join(promptsDir, "001-empty-move-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		// Empty body triggers ErrEmptyPrompt from pf.Content()
		mockManager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte(""),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(stderrors.New("move failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// MoveToCompleted was attempted
		Eventually(func() int {
			return mockManager.MoveToCompletedCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Executor was NOT called (prompt was empty)
		Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

		// Load should be called to mark prompt as failed (MoveToCompleted returned error)
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when Process fails to reset failed prompts", func() {
		mockManager.ResetFailedReturns(stderrors.New("reset failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		err := p.Process(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("reset failed prompts"))
	})

	It("should return error when ListQueued fails during startup scan", func() {
		mockManager.ListQueuedReturns(nil, stderrors.New("list queued failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		err := p.Process(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list queued prompts"))
	})

	It("should return error when CommitCompletedFile fails", func() {
		promptPath := filepath.Join(promptsDir, "001-commitfile-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(stderrors.New("commit completed file failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Load called to mark as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Executor was called but CommitCompletedFile failed
		Expect(mockExecutor.ExecuteCallCount()).To(Equal(1))

		cancel()
	})

	It("should return error when Switch to default branch fails in postMergeActions", func() {
		originalDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = os.Chdir(originalDir)
		})

		promptPath := filepath.Join(promptsDir, "001-switch-default-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		mockCloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.CommitOnlyReturns(nil)
		mockBrancher.PushReturns(nil)
		mockPRCreator.CreateReturns("https://github.com/test/pull/1", nil)
		mockManager.SetPRURLReturns(nil)
		mockPRMerger.WaitAndMergeReturns(nil)
		mockBrancher.DefaultBranchReturns("main", nil)
		// Switch fails after DefaultBranch succeeds
		mockBrancher.SwitchReturns(stderrors.New("switch to default branch failed"))

		// Create log file with success report
		logDir := filepath.Join(promptsDir, "log")
		_ = os.MkdirAll(logDir, 0750)
		logPath := filepath.Join(logDir, "001-switch-default-fail.log")
		_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			logDir,
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			true,
			true,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			true, // autoMerge enabled
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// DefaultBranch and Switch are called
		Eventually(func() int {
			return mockBrancher.DefaultBranchCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when Pull fails in postMergeActions", func() {
		originalDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = os.Chdir(originalDir)
		})

		promptPath := filepath.Join(promptsDir, "001-pull-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		mockCloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.CommitOnlyReturns(nil)
		mockBrancher.PushReturns(nil)
		mockPRCreator.CreateReturns("https://github.com/test/pull/1", nil)
		mockManager.SetPRURLReturns(nil)
		mockPRMerger.WaitAndMergeReturns(nil)
		mockBrancher.DefaultBranchReturns("main", nil)
		mockBrancher.SwitchReturns(nil)
		// Pull fails after Switch succeeds
		mockBrancher.PullReturns(stderrors.New("pull failed"))

		// Create log file with success report
		logDir := filepath.Join(promptsDir, "log")
		_ = os.MkdirAll(logDir, 0750)
		logPath := filepath.Join(logDir, "001-pull-fail.log")
		_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			logDir,
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			true,
			true,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			true, // autoMerge enabled
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Pull is called and fails
		Eventually(func() int {
			return mockBrancher.PullCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when CommitOnly fails in direct workflow (no changelog)", func() {
		promptPath := filepath.Join(promptsDir, "001-commitonly-nochangelog-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.HasChangelogReturns(false)
		mockReleaser.CommitOnlyReturns(stderrors.New("commit only failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			false,
			false,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			false,
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// CommitOnly was called and failed
		Eventually(func() int {
			return mockReleaser.CommitOnlyCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when DefaultBranch fails after successful merge", func() {
		originalDir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = os.Chdir(originalDir)
		})

		promptPath := filepath.Join(promptsDir, "001-defaultbranch-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		mockCloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		mockManager.ListQueuedReturnsOnCall(0, queued, nil)
		mockManager.ListQueuedReturnsOnCall(1, nil, nil)
		mockManager.AllPreviousCompletedReturns(true)
		mockManager.MoveToCompletedReturns(nil)
		mockExecutor.ExecuteReturns(nil)
		mockReleaser.CommitCompletedFileReturns(nil)
		mockReleaser.CommitOnlyReturns(nil)
		mockBrancher.PushReturns(nil)
		mockPRCreator.CreateReturns("https://github.com/test/pull/1", nil)
		mockManager.SetPRURLReturns(nil)
		mockPRMerger.WaitAndMergeReturns(nil)
		// DefaultBranch fails after successful merge
		mockBrancher.DefaultBranchReturns("", stderrors.New("cannot determine default branch"))

		// Create log file with success report
		logDir := filepath.Join(promptsDir, "log")
		_ = os.MkdirAll(logDir, 0750)
		logPath := filepath.Join(logDir, "001-defaultbranch-fail.log")
		_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			logDir,
			"test-project",
			mockExecutor,
			mockManager,
			mockReleaser,
			mockVersionGet,
			ready,
			true,
			true,
			mockBrancher,
			mockPRCreator,
			mockCloner,
			mockPRMerger,
			true, // autoMerge enabled
			false,
			false,
			mockAutoCompleter,
			mockSpecLister,
			"",
			false,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// WaitAndMerge is called
		Eventually(func() int {
			return mockPRMerger.WaitAndMergeCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// DefaultBranch is called and fails → prompt marked as failed
		Eventually(func() int {
			return mockBrancher.DefaultBranchCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return mockManager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	Context("Auto-merge", func() {
		It("should not call WaitAndMerge when autoMerge is false (PR workflow)", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-no-automerge.md")
			// completedPath not needed
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(nil)
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.SetPRURLReturns(nil)

			// Create log file with success report
			logDir := filepath.Join(promptsDir, "log")
			_ = os.MkdirAll(logDir, 0750)
			logPath := filepath.Join(logDir, "001-no-automerge.log")
			_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Auto-merge test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false, // autoMerge disabled
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for PR to be created
			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// WaitAndMerge should NOT be called
			Consistently(func() int {
				return mockPRMerger.WaitAndMergeCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Worktree should be removed after PR creation
			Eventually(func() int {
				return mockCloner.RemoveCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})

		It(
			"should call WaitAndMerge when autoMerge is true and merge succeeds (PR workflow)",
			func() {
				originalDir, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					_ = os.Chdir(originalDir)
				})

				promptPath := filepath.Join(promptsDir, "001-automerge.md")
				// completedPath not needed
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				mockCloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.CommitOnlyReturns(nil)
				mockBrancher.FetchReturns(nil)
				mockBrancher.MergeOriginDefaultReturns(nil)
				mockBrancher.PushReturns(nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.SwitchReturns(nil)
				mockBrancher.PullReturns(nil)
				mockPRCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
				mockPRMerger.WaitAndMergeReturns(nil)
				mockManager.MoveToCompletedReturns(nil)
				mockManager.SetPRURLReturns(nil)
				mockReleaser.HasChangelogReturns(false)

				// Create log file with success report
				logDir := filepath.Join(promptsDir, "log")
				_ = os.MkdirAll(logDir, 0750)
				logPath := filepath.Join(logDir, "001-automerge.log")
				_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Auto-merge test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					logDir,
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					true,
					true,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					true, // autoMerge enabled
					false,
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					false,
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for WaitAndMerge to be called
				Eventually(func() int {
					return mockPRMerger.WaitAndMergeCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// Verify DefaultBranch was called
				Eventually(func() int {
					return mockBrancher.DefaultBranchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// Verify Switch to default branch was called
				Eventually(func() int {
					return mockBrancher.SwitchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				// Verify Pull was called
				Eventually(func() int {
					return mockBrancher.PullCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				cancel()
			},
		)

		It("should call CommitAndRelease when autoRelease is true and changelog exists", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-autorelease.md")
			// completedPath not needed
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(nil)
			mockBrancher.PushReturns(nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.SwitchReturns(nil)
			mockBrancher.PullReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			mockPRMerger.WaitAndMergeReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.SetPRURLReturns(nil)
			mockReleaser.HasChangelogReturns(true)
			mockReleaser.GetNextVersionReturns("v0.0.2", nil)
			mockReleaser.CommitAndReleaseReturns(nil)

			// Create log file with success report
			logDir := filepath.Join(promptsDir, "log")
			_ = os.MkdirAll(logDir, 0750)
			logPath := filepath.Join(logDir, "001-autorelease.log")
			_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Auto-release test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				true, // autoMerge enabled
				true, // autoRelease enabled
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for CommitAndRelease to be called
			Eventually(func() int {
				return mockReleaser.CommitAndReleaseCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})

		It(
			"should not call CommitAndRelease when autoRelease is true but no changelog exists",
			func() {
				originalDir, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					_ = os.Chdir(originalDir)
				})

				promptPath := filepath.Join(promptsDir, "001-no-changelog.md")
				// completedPath not needed
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				mockCloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.CommitOnlyReturns(nil)
				mockBrancher.FetchReturns(nil)
				mockBrancher.MergeOriginDefaultReturns(nil)
				mockBrancher.PushReturns(nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.SwitchReturns(nil)
				mockBrancher.PullReturns(nil)
				mockPRCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
				mockPRMerger.WaitAndMergeReturns(nil)
				mockManager.MoveToCompletedReturns(nil)
				mockManager.SetPRURLReturns(nil)
				mockReleaser.HasChangelogReturns(false) // No changelog

				// Create log file with success report
				logDir := filepath.Join(promptsDir, "log")
				_ = os.MkdirAll(logDir, 0750)
				logPath := filepath.Join(logDir, "001-no-changelog.log")
				_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"No changelog test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					logDir,
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					true,
					true,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					true, // autoMerge enabled
					true, // autoRelease enabled
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					false,
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for WaitAndMerge to complete
				Eventually(func() int {
					return mockPRMerger.WaitAndMergeCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// CommitAndRelease should NOT be called
				Consistently(func() int {
					return mockReleaser.CommitAndReleaseCallCount()
				}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

				cancel()
			},
		)

		It("should not call CommitAndRelease when autoRelease is false", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-no-autorelease.md")
			// completedPath not needed
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(nil)
			mockBrancher.PushReturns(nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.SwitchReturns(nil)
			mockBrancher.PullReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			mockPRMerger.WaitAndMergeReturns(nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.SetPRURLReturns(nil)
			mockReleaser.HasChangelogReturns(true) // Changelog exists but autoRelease is false

			// Create log file with success report
			logDir := filepath.Join(promptsDir, "log")
			_ = os.MkdirAll(logDir, 0750)
			logPath := filepath.Join(logDir, "001-no-autorelease.log")
			_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"No autorelease test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				true,  // autoMerge enabled
				false, // autoRelease disabled
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to complete
			Eventually(func() int {
				return mockPRMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// CommitAndRelease should NOT be called
			Consistently(func() int {
				return mockReleaser.CommitAndReleaseCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			cancel()
		})
	})

	Context("Auto-review", func() {
		It(
			"should set status to in_review and NOT move to completed when autoReview=true (PR workflow)",
			func() {
				originalDir, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() {
					_ = os.Chdir(originalDir)
				})

				promptPath := filepath.Join(promptsDir, "001-auto-review.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				mockCloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitOnlyReturns(nil)
				mockBrancher.FetchReturns(nil)
				mockBrancher.MergeOriginDefaultReturns(nil)
				mockBrancher.PushReturns(nil)
				mockPRCreator.CreateReturns("https://github.com/test/test/pull/42", nil)
				mockManager.SetStatusReturns(nil)
				mockManager.SetPRURLReturns(nil)

				// Create log file with success report
				logDir := filepath.Join(promptsDir, "log")
				_ = os.MkdirAll(logDir, 0750)
				logPath := filepath.Join(logDir, "001-auto-review.log")
				_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Auto-review test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					logDir,
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					true,
					true,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					false, // autoMerge disabled
					false,
					true, // autoReview enabled
					mockAutoCompleter,
					mockSpecLister,
					"",
					false,
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for PR to be created
				Eventually(func() int {
					return mockPRCreator.CreateCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// SetStatus should be called with in_review
				Eventually(func() string {
					if mockManager.SetStatusCallCount() == 0 {
						return ""
					}
					_, _, status := mockManager.SetStatusArgsForCall(
						mockManager.SetStatusCallCount() - 1,
					)
					return status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(string(prompt.InReviewPromptStatus)))

				// MoveToCompleted should NOT be called
				Consistently(func() int {
					return mockManager.MoveToCompletedCallCount()
				}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

				// WaitAndMerge should NOT be called
				Consistently(func() int {
					return mockPRMerger.WaitAndMergeCallCount()
				}, 200*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

				cancel()
			},
		)

		It("should move to completed normally when autoReview=false (PR workflow)", func() {
			originalDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = os.Chdir(originalDir)
			})

			promptPath := filepath.Join(promptsDir, "001-no-auto-review.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.FetchReturns(nil)
			mockBrancher.MergeOriginDefaultReturns(nil)
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/test/test/pull/43", nil)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.SetPRURLReturns(nil)

			// Create log file with success report
			logDir := filepath.Join(promptsDir, "log")
			_ = os.MkdirAll(logDir, 0750)
			logPath := filepath.Join(logDir, "001-no-auto-review.log")
			_ = os.WriteFile(logPath, []byte(`<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"No auto-review test","blockers":[]}
DARK-FACTORY-REPORT -->`), 0600)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true,
				true,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false, // autoMerge disabled
				false,
				false, // autoReview disabled
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for PR to be created
			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// MoveToCompleted should be called (normal flow)
			Eventually(func() int {
				return mockManager.MoveToCompletedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// SetStatus should NOT be called with in_review
			Consistently(func() bool {
				for i := 0; i < mockManager.SetStatusCallCount(); i++ {
					_, _, status := mockManager.SetStatusArgsForCall(i)
					if status == string(prompt.InReviewPromptStatus) {
						return true
					}
				}
				return false
			}, 500*time.Millisecond, 50*time.Millisecond).Should(BeFalse())

			cancel()
		})
	})

	Describe("Verification Gate", func() {
		var promptPath string

		BeforeEach(func() {
			promptPath = filepath.Join(promptsDir, "001-gate-test.md")
			// Override Load stub to use real file I/O so pf.Save works
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			}
		})

		Context("gate enabled, execution succeeds", func() {
			It("enters pending_verification and does not call MoveToCompleted", func() {
				Expect(os.WriteFile(
					promptPath,
					[]byte("---\nstatus: approved\n---\n# Gate Test\n\nContent\n"),
					0600,
				)).To(Succeed())

				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.HasChangelogReturns(false)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					false,
					false,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					false,
					false,
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					true, // verificationGate enabled
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				Eventually(func() int {
					return mockExecutor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// MoveToCompleted must NOT be called — git ops are deferred
				Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))

				// File status must be pending_verification
				// Use Eventually to avoid a race: enterPendingVerification runs
				// asynchronously after Execute returns in the processor goroutine.
				Eventually(func() string {
					pf, loadErr := prompt.Load(ctx, promptPath, libtime.NewCurrentDateTime())
					if loadErr != nil {
						return ""
					}
					return pf.Frontmatter.Status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(string(prompt.PendingVerificationPromptStatus)))

				cancel()
				<-errCh
			})
		})

		Context("gate enabled, execution fails", func() {
			It("marks prompt failed without entering pending_verification", func() {
				Expect(os.WriteFile(
					promptPath,
					[]byte("---\nstatus: approved\n---\n# Gate Test\n\nContent\n"),
					0600,
				)).To(Succeed())

				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockExecutor.ExecuteReturns(stderrors.New("execution failed"))

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					false,
					false,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					false,
					false,
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					true, // verificationGate enabled
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				Eventually(func() int {
					return mockExecutor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// MoveToCompleted must NOT be called
				Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))

				// Status must be failed — gate does not apply to failed executions
				pf, loadErr := prompt.Load(ctx, promptPath, libtime.NewCurrentDateTime())
				Expect(loadErr).NotTo(HaveOccurred())
				Expect(pf.Frontmatter.Status).To(Equal(string(prompt.FailedPromptStatus)))

				cancel()
				<-errCh
			})
		})

		Context("hasPendingVerification blocks queue", func() {
			It(
				"returns nil without calling ListQueued when pending_verification prompt exists",
				func() {
					Expect(os.WriteFile(
						promptPath,
						[]byte("---\nstatus: pending_verification\n---\n# Pending\n\nContent\n"),
						0600,
					)).To(Succeed())

					p := processor.NewProcessor(
						promptsDir,
						filepath.Join(promptsDir, "completed"),
						filepath.Join(promptsDir, "log"),
						"test-project",
						mockExecutor,
						mockManager,
						mockReleaser,
						mockVersionGet,
						ready,
						false,
						false,
						mockBrancher,
						mockPRCreator,
						mockCloner,
						mockPRMerger,
						false,
						false,
						false,
						mockAutoCompleter,
						mockSpecLister,
						"",
						false,
					)

					errCh := make(chan error, 1)
					go func() {
						errCh <- p.Process(ctx)
					}()

					// Give the processor time to run the initial scan
					time.Sleep(200 * time.Millisecond)

					// ListQueued must NOT be called — queue is blocked before the loop
					Expect(mockManager.ListQueuedCallCount()).To(Equal(0))
					// Executor must NOT be called
					Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))

					cancel()
					<-errCh
				},
			)
		})

		Context("hasPendingVerification false, queue proceeds normally", func() {
			It("calls ListQueued when no pending_verification prompt exists", func() {
				mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					false,
					false,
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					false,
					false,
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					true, // gate enabled but no pending file
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				// ListQueued should be called (no pending file to block)
				Eventually(func() int {
					return mockManager.ListQueuedCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				cancel()
				<-errCh
			})
		})
	})

	Describe("startup checkPromptedSpecs", func() {
		It("should call CheckAndComplete for specs in prompted status on startup", func() {
			// Create a spec file in prompted status
			specDir := filepath.Join(tempDir, "specs")
			Expect(os.MkdirAll(specDir, 0750)).To(Succeed())
			specPath := filepath.Join(specDir, "001-my-spec.md")
			Expect(
				os.WriteFile(specPath, []byte("---\nstatus: prompted\n---\n# My Spec\n"), 0600),
			).To(Succeed())

			// Use a real lister pointing at the spec dir
			realLister := spec.NewLister(specDir)

			mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)
			mockAutoCompleter.CheckAndCompleteReturns(nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				realLister,
				"",
				false,
			)

			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for CheckAndComplete to be called with the spec name
			Eventually(func() int {
				return mockAutoCompleter.CheckAndCompleteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, specID := mockAutoCompleter.CheckAndCompleteArgsForCall(0)
			Expect(specID).To(Equal("001-my-spec"))

			cancel()
		})
	})

	Describe("ProcessQueue log output", func() {
		var (
			logBuf      bytes.Buffer
			origDefault *slog.Logger
		)

		BeforeEach(func() {
			logBuf.Reset()
			origDefault = slog.Default()
			slog.SetDefault(
				slog.New(
					slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}),
				),
			)
		})

		AfterEach(func() {
			slog.SetDefault(origDefault)
		})

		newProc := func() processor.Processor {
			return processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				false,
				false,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)
		}

		It("ProcessQueue logs 'no queued prompts' once when queue is empty", func() {
			mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).NotTo(HaveOccurred())

			output := logBuf.String()
			Expect(strings.Count(output, "no queued prompts")).To(Equal(1))
		})

		It("ProcessQueue does not log 'no queued prompts, exiting'", func() {
			mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(logBuf.String()).NotTo(ContainSubstring("no queued prompts, exiting"))
		})

		It("daemon Process logs 'waiting for changes' once after startup scan", func() {
			mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			time.Sleep(200 * time.Millisecond)
			cancel()

			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
			case <-time.After(2 * time.Second):
				Fail("processor did not stop within timeout")
			}

			Expect(logBuf.String()).To(ContainSubstring("waiting for changes"))
			Expect(strings.Count(logBuf.String(), "waiting for changes")).To(Equal(1))
		})

		It("daemon ticker scan does not log 'no queued prompts' at INFO level", func() {
			mockManager.ListQueuedReturns([]prompt.Prompt{}, nil)

			// Use INFO-only handler to verify no INFO logs for empty queue scans
			var infoBuf bytes.Buffer
			slog.SetDefault(
				slog.New(
					slog.NewTextHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo}),
				),
			)

			p := newProc()
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			time.Sleep(200 * time.Millisecond)
			cancel()

			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
			case <-time.After(2 * time.Second):
				Fail("processor did not stop within timeout")
			}

			Expect(infoBuf.String()).NotTo(ContainSubstring("no queued prompts"))
		})
	})

	Describe("In-place branch switching", func() {
		createBranchPromptFile := func(path string, branch string) *prompt.PromptFile {
			return prompt.NewPromptFile(
				path,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus), Branch: branch},
				[]byte("# Test\n\nDefault test content"),
				libtime.NewCurrentDateTime(),
			)
		}

		newProcWithWorktree := func(pr bool, worktree bool) processor.Processor {
			return processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				pr,
				worktree,
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)
		}

		It("worktree=false, branch='': no branch switch called", func() {
			promptPath := filepath.Join(promptsDir, "001-no-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			p := newProcWithWorktree(false, false)
			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return mockExecutor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			Expect(mockBrancher.IsCleanCallCount()).To(Equal(0))
			Expect(mockBrancher.SwitchCallCount()).To(Equal(0))
			Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(0))

			cancel()
		})

		It(
			"worktree=false, branch set, clean, branch exists remotely: Switch called not CreateAndSwitch",
			func() {
				promptPath := filepath.Join(promptsDir, "001-branch-exists.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/test"), nil
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockManager.MoveToCompletedReturns(nil)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.HasChangelogReturns(false)
				mockReleaser.CommitOnlyReturns(nil)

				mockBrancher.IsCleanReturns(true, nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.FetchAndVerifyBranchReturns(nil) // branch exists remotely
				mockBrancher.SwitchReturns(nil)

				p := newProcWithWorktree(false, false)
				go func() {
					_ = p.Process(ctx)
				}()

				Eventually(func() int {
					return mockExecutor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(mockBrancher.IsCleanCallCount()).To(Equal(1))
				Expect(mockBrancher.FetchAndVerifyBranchCallCount()).To(Equal(1))
				_, branchArg := mockBrancher.FetchAndVerifyBranchArgsForCall(0)
				Expect(branchArg).To(Equal("dark-factory/test"))
				// Switch called: once to switch to branch, once to restore default
				Expect(mockBrancher.SwitchCallCount()).To(BeNumerically(">=", 1))
				_, switchArg := mockBrancher.SwitchArgsForCall(0)
				Expect(switchArg).To(Equal("dark-factory/test"))
				Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(0))

				cancel()
			},
		)

		It(
			"worktree=false, branch set, clean, branch not on remote: CreateAndSwitch called",
			func() {
				promptPath := filepath.Join(promptsDir, "001-branch-new.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/test"), nil
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockManager.MoveToCompletedReturns(nil)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.HasChangelogReturns(false)
				mockReleaser.CommitOnlyReturns(nil)

				mockBrancher.IsCleanReturns(true, nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.FetchAndVerifyBranchReturns(
					stderrors.New("branch not found"),
				) // branch does not exist
				mockBrancher.CreateAndSwitchReturns(nil)
				mockBrancher.SwitchReturns(nil)

				p := newProcWithWorktree(false, false)
				go func() {
					_ = p.Process(ctx)
				}()

				Eventually(func() int {
					return mockExecutor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(mockBrancher.IsCleanCallCount()).To(Equal(1))
				Expect(mockBrancher.FetchAndVerifyBranchCallCount()).To(Equal(1))
				Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(1))
				_, createArg := mockBrancher.CreateAndSwitchArgsForCall(0)
				Expect(createArg).To(Equal("dark-factory/test"))

				cancel()
			},
		)

		It("worktree=false, branch set, dirty tree: returns error, no branch operation", func() {
			promptPath := filepath.Join(promptsDir, "001-dirty.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "dark-factory/test"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)

			mockBrancher.IsCleanReturns(false, nil) // dirty working tree

			p := newProcWithWorktree(false, false)
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for the IsClean call to happen (prompt fails)
			Eventually(func() int {
				return mockBrancher.IsCleanCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			Expect(mockExecutor.ExecuteCallCount()).To(Equal(0))
			Expect(mockBrancher.SwitchCallCount()).To(Equal(0))
			Expect(mockBrancher.CreateAndSwitchCallCount()).To(Equal(0))

			cancel()
		})

		It("worktree=true, branch set: uses clone workflow, not in-place", func() {
			promptPath := filepath.Join(promptsDir, "001-clone-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "dark-factory/test"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/456", nil)

			p := newProcWithWorktree(true, true)
			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return mockCloner.CloneCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// In-place branch switching should NOT have been called
			Expect(mockBrancher.IsCleanCallCount()).To(Equal(0))

			cancel()
		})

		It("restores default branch after direct workflow with in-place branch", func() {
			promptPath := filepath.Join(promptsDir, "001-restore.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "dark-factory/restore-test"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			// Simulate more prompts on branch so merge/release is skipped
			mockManager.HasQueuedPromptsOnBranchReturns(true, nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)

			mockBrancher.IsCleanReturns(true, nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
			mockBrancher.CreateAndSwitchReturns(nil)
			mockBrancher.SwitchReturns(nil)

			p := newProcWithWorktree(false, false)
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for execution to complete
			Eventually(func() int {
				return mockReleaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Switch should be called: once to restore default branch
			Expect(mockBrancher.SwitchCallCount()).To(BeNumerically(">=", 1))
			// Find the restore call (last Switch call should be to "main")
			lastIdx := mockBrancher.SwitchCallCount() - 1
			_, lastSwitchArg := mockBrancher.SwitchArgsForCall(lastIdx)
			Expect(lastSwitchArg).To(Equal("main"))

			cancel()
		})
	})

	Describe("Release guard on feature branches", func() {
		createBranchPromptFile := func(path string, branch string) *prompt.PromptFile {
			return prompt.NewPromptFile(
				path,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus), Branch: branch},
				[]byte("# Test\n\nDefault test content"),
				libtime.NewCurrentDateTime(),
			)
		}

		var newProcDirect func() processor.Processor
		BeforeEach(func() {
			newProcDirect = func() processor.Processor {
				return processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					mockExecutor,
					mockManager,
					mockReleaser,
					mockVersionGet,
					ready,
					false, // pr=false
					false, // worktree=false
					mockBrancher,
					mockPRCreator,
					mockCloner,
					mockPRMerger,
					false,
					false,
					false,
					mockAutoCompleter,
					mockSpecLister,
					"",
					false,
				)
			}
		})

		It(
			"feature branch: calls CommitOnly (not CommitAndRelease) even when changelog exists",
			func() {
				promptPath := filepath.Join(promptsDir, "001-fb-commit.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "feature/my-branch"), nil
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockManager.MoveToCompletedReturns(nil)
				// More prompts on branch — skip merge
				mockManager.HasQueuedPromptsOnBranchReturns(true, nil)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.HasChangelogReturns(true) // changelog exists but should NOT release
				mockReleaser.CommitOnlyReturns(nil)

				mockBrancher.IsCleanReturns(true, nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
				mockBrancher.CreateAndSwitchReturns(nil)
				mockBrancher.SwitchReturns(nil)

				p := newProcDirect()
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return mockReleaser.CommitOnlyCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// CommitAndRelease must NOT be called
				Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))

				cancel()
			},
		)

		It("default branch with changelog: CommitAndRelease called (unchanged behavior)", func() {
			promptPath := filepath.Join(promptsDir, "001-default-changelog.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(true)
			mockReleaser.GetNextVersionReturns("v0.1.1", nil)
			mockReleaser.CommitAndReleaseReturns(nil)

			p := newProcDirect()
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockReleaser.CommitAndReleaseCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// CommitOnly must NOT be called
			Expect(mockReleaser.CommitOnlyCallCount()).To(Equal(0))

			cancel()
		})

		It("handleBranchCompletion: HasQueuedPromptsOnBranch=true skips MergeToDefault", func() {
			promptPath := filepath.Join(promptsDir, "001-has-more.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "feature/shared"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			// More prompts on same branch
			mockManager.HasQueuedPromptsOnBranchReturns(true, nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(false)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.IsCleanReturns(true, nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
			mockBrancher.CreateAndSwitchReturns(nil)
			mockBrancher.SwitchReturns(nil)

			p := newProcDirect()
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockManager.HasQueuedPromptsOnBranchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// MergeToDefault must NOT be called
			Expect(mockBrancher.MergeToDefaultCallCount()).To(Equal(0))

			cancel()
		})

		It(
			"handleBranchCompletion: HasQueuedPromptsOnBranch=false triggers MergeToDefault and CommitAndRelease",
			func() {
				promptPath := filepath.Join(promptsDir, "001-last-on-branch.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "feature/last"), nil
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mockManager.AllPreviousCompletedReturns(true)
				mockManager.MoveToCompletedReturns(nil)
				// No more prompts on branch — trigger merge
				mockManager.HasQueuedPromptsOnBranchReturns(false, nil)
				mockExecutor.ExecuteReturns(nil)
				mockReleaser.CommitCompletedFileReturns(nil)
				mockReleaser.HasChangelogReturns(true)
				mockReleaser.GetNextVersionReturns("v0.2.0", nil)
				mockReleaser.CommitAndReleaseReturns(nil)
				mockReleaser.CommitOnlyReturns(nil)
				mockBrancher.IsCleanReturns(true, nil)
				mockBrancher.DefaultBranchReturns("main", nil)
				mockBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
				mockBrancher.CreateAndSwitchReturns(nil)
				mockBrancher.SwitchReturns(nil)
				mockBrancher.MergeToDefaultReturns(nil)

				p := newProcDirect()
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return mockBrancher.MergeToDefaultCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Eventually(func() int {
					return mockReleaser.CommitAndReleaseCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				cancel()
			},
		)

		It("handleBranchCompletion: MergeToDefault error stops release", func() {
			promptPath := filepath.Join(promptsDir, "001-merge-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "feature/conflict"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockManager.HasQueuedPromptsOnBranchReturns(false, nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.HasChangelogReturns(true)
			mockReleaser.CommitOnlyReturns(nil)
			mockBrancher.IsCleanReturns(true, nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
			mockBrancher.CreateAndSwitchReturns(nil)
			mockBrancher.SwitchReturns(nil)
			mockBrancher.MergeToDefaultReturns(stderrors.New("merge conflict"))

			p := newProcDirect()
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockBrancher.MergeToDefaultCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Release must NOT be called after merge failure
			Expect(mockReleaser.CommitAndReleaseCallCount()).To(Equal(0))

			cancel()
		})

		It("pr=true: handleBranchCompletion NOT called even when feature branch completes", func() {
			promptPath := filepath.Join(promptsDir, "001-pr-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			// pr=true with worktree=true uses clone workflow
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "feature/pr-mode"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/789", nil)

			// pr=true, worktree=true
			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true, // pr=true
				true, // worktree=true
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				false,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// handleBranchCompletion must NOT be called (pr=true uses clone workflow)
			Expect(mockManager.HasQueuedPromptsOnBranchCallCount()).To(Equal(0))
			Expect(mockBrancher.MergeToDefaultCallCount()).To(Equal(0))

			cancel()
		})
	})

	Describe("Idempotent PR creation and deferred auto-merge", func() {
		createIssuePromptFile := func(path string, branch string, issue string) *prompt.PromptFile {
			return prompt.NewPromptFile(
				path,
				prompt.Frontmatter{
					Status: string(prompt.ApprovedPromptStatus),
					Branch: branch,
					Issue:  issue,
				},
				[]byte("# Test\n\nDefault test content"),
				libtime.NewCurrentDateTime(),
			)
		}

		setupCloneMocks := func() {
			mockCloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			mockCloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			mockBrancher.PushReturns(nil)
			mockManager.AllPreviousCompletedReturns(true)
			mockManager.MoveToCompletedReturns(nil)
			mockExecutor.ExecuteReturns(nil)
			mockReleaser.CommitCompletedFileReturns(nil)
			mockReleaser.CommitOnlyReturns(nil)
		}

		newProcWorktree := func(autoMerge bool) processor.Processor {
			return processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				mockExecutor,
				mockManager,
				mockReleaser,
				mockVersionGet,
				ready,
				true, // pr=true
				true, // worktree=true
				mockBrancher,
				mockPRCreator,
				mockCloner,
				mockPRMerger,
				autoMerge,
				false,
				false,
				mockAutoCompleter,
				mockSpecLister,
				"",
				false,
			)
		}

		It("FindOpenPR returns existing URL: Create NOT called, existing URL used", func() {
			promptPath := filepath.Join(promptsDir, "001-idempotent.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/idempotent", ""), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			// Existing PR found
			mockPRCreator.FindOpenPRReturns("https://github.com/user/repo/pull/42", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.FindOpenPRCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Create must NOT be called when PR already exists
			Expect(mockPRCreator.CreateCallCount()).To(Equal(0))

			cancel()
		})

		It("FindOpenPR returns empty: Create called with buildPRBody result", func() {
			promptPath := filepath.Join(promptsDir, "001-create-pr.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/new-pr", ""), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			// No existing PR
			mockPRCreator.FindOpenPRReturns("", nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/99", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify body is the default (no issue)
			_, _, body := mockPRCreator.CreateArgsForCall(0)
			Expect(body).To(Equal("Automated by dark-factory"))

			cancel()
		})

		It("pf.Issue() non-empty: PR body contains issue reference", func() {
			promptPath := filepath.Join(promptsDir, "001-with-issue.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/issue-ref", "BRO-42"), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			mockPRCreator.FindOpenPRReturns("", nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/100", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, _, body := mockPRCreator.CreateArgsForCall(0)
			Expect(body).To(ContainSubstring("Issue: BRO-42"))
			Expect(body).To(Equal("Automated by dark-factory\n\nIssue: BRO-42"))

			cancel()
		})

		It("pf.Issue() empty: PR body is default without issue line", func() {
			promptPath := filepath.Join(promptsDir, "001-no-issue.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/no-issue", ""), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			mockPRCreator.FindOpenPRReturns("", nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/101", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, _, body := mockPRCreator.CreateArgsForCall(0)
			Expect(body).To(Equal("Automated by dark-factory"))
			Expect(body).NotTo(ContainSubstring("Issue:"))

			cancel()
		})

		It(
			"autoMerge=true, HasQueuedPromptsOnBranch=true: WaitAndMerge NOT called, moved to completed",
			func() {
				promptPath := filepath.Join(promptsDir, "001-defer-merge.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}
				mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createIssuePromptFile(path, "feature/multi", ""), nil
				}
				mockManager.ListQueuedReturnsOnCall(0, queued, nil)
				mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				setupCloneMocks()
				mockPRCreator.FindOpenPRReturns("https://github.com/user/repo/pull/50", nil)
				// More prompts on branch — defer merge
				mockManager.HasQueuedPromptsOnBranchReturns(true, nil)

				p := newProcWorktree(true) // autoMerge=true
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return mockManager.HasQueuedPromptsOnBranchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				// WaitAndMerge must NOT be called
				Expect(mockPRMerger.WaitAndMergeCallCount()).To(Equal(0))
				// Prompt must be moved to completed
				Expect(mockManager.MoveToCompletedCallCount()).To(Equal(1))

				cancel()
			},
		)

		It("autoMerge=true, HasQueuedPromptsOnBranch=false: WaitAndMerge called", func() {
			promptPath := filepath.Join(promptsDir, "001-last-prompt.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/last", ""), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			mockPRCreator.FindOpenPRReturns("https://github.com/user/repo/pull/51", nil)
			// Last prompt on branch — trigger merge
			mockManager.HasQueuedPromptsOnBranchReturns(false, nil)
			mockPRMerger.WaitAndMergeReturns(nil)
			mockBrancher.DefaultBranchReturns("main", nil)
			mockBrancher.SwitchReturns(nil)
			mockBrancher.PullReturns(nil)
			mockReleaser.HasChangelogReturns(false)

			p := newProcWorktree(true) // autoMerge=true
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})

		It("autoMerge=false: PR created but WaitAndMerge NOT called", func() {
			promptPath := filepath.Join(promptsDir, "001-no-merge.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			mockManager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/no-automerge", ""), nil
			}
			mockManager.ListQueuedReturnsOnCall(0, queued, nil)
			mockManager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			mockPRCreator.FindOpenPRReturns("", nil)
			mockPRCreator.CreateReturns("https://github.com/user/repo/pull/102", nil)

			p := newProcWorktree(false) // autoMerge=false
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return mockPRCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// WaitAndMerge must NOT be called
			Expect(mockPRMerger.WaitAndMergeCallCount()).To(Equal(0))

			cancel()
		})
	})
})
