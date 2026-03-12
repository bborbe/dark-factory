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
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var _ = Describe("Processor", func() {
	var (
		tempDir       string
		promptsDir    string
		ready         chan struct{}
		ctx           context.Context
		cancel        context.CancelFunc
		executor      *mocks.Executor
		manager       *mocks.Manager
		releaser      *mocks.Releaser
		versionGet    *mocks.VersionGetter
		brancher      *mocks.Brancher
		prCreator     *mocks.PRCreator
		cloner        *mocks.Cloner
		prMerger      *mocks.PRMerger
		autoCompleter *mocks.AutoCompleter
		specLister    *mocks.Lister
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

		executor = &mocks.Executor{}
		manager = &mocks.Manager{}
		releaser = &mocks.Releaser{}
		versionGet = &mocks.VersionGetter{}
		brancher = &mocks.Brancher{}
		prCreator = &mocks.PRCreator{}
		cloner = &mocks.Cloner{}
		prMerger = &mocks.PRMerger{}
		autoCompleter = &mocks.AutoCompleter{}
		specLister = &mocks.Lister{}
		specLister.ListReturns(nil, nil)
		versionGet.GetReturns("v0.0.1-test")
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
		manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
			return createMockPromptFile(path, "# Test\n\nDefault test content"), nil
		}
	})

	It("should start and stop cleanly", func() {
		manager.ListQueuedReturns([]prompt.Prompt{}, nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
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
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.ContentReturns("# Test prompt", nil)
		manager.TitleReturns("Test prompt", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was called with correct log path
		_, _, logFile, containerName := executor.ExecuteArgsForCall(0)
		Expect(logFile).To(Equal(filepath.Join(promptsDir, "log", "001-test.log")))
		Expect(containerName).To(Equal("test-project-001-test"))

		// Verify Load was called (processor uses Load/Save pattern now)
		Expect(manager.LoadCallCount()).To(BeNumerically(">=", 1))

		// Verify moved to completed
		Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

		cancel()
	})

	It("should process prompts when ready signal received", func() {
		promptPath := filepath.Join(promptsDir, "001-signal.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		// Initially no prompts
		manager.ListQueuedReturns([]prompt.Prompt{}, nil)
		manager.ContentReturns("# Signal test", nil)
		manager.TitleReturns("Signal test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for initial scan
		time.Sleep(200 * time.Millisecond)

		// Now return a prompt and send ready signal
		manager.ListQueuedReturnsOnCall(1, queued, nil)
		manager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)
		ready <- struct{}{}

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		cancel()
	})

	It("should skip empty prompts", func() {
		promptPath := filepath.Join(promptsDir, "001-empty.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return empty body
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte(""),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.ContentReturns("", prompt.ErrEmptyPrompt)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return manager.MoveToCompletedCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should handle executor errors and mark prompt as failed", func() {
		promptPath := filepath.Join(promptsDir, "001-fail.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		// Return queued once, then empty (so loop exits after failure)
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.ContentReturns("# Fail test", nil)
		manager.TitleReturns("Fail test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(stderrors.New("execution failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor — marks failed and continues (no error returned)
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for Load to be called (marks prompt as failed)
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should fire prompt_failed notification when executor returns error", func() {
		promptPath := filepath.Join(promptsDir, "001-fail-notify.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.ContentReturns("# Fail notify test", nil)
		manager.TitleReturns("Fail notify test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(stderrors.New("execution failed"))

		fakeNotifier := &mocks.Notifier{}

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			fakeNotifier,
		)

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return fakeNotifier.NotifyCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))
		cancel()

		_, event := fakeNotifier.NotifyArgsForCall(0)
		Expect(event.EventType).To(Equal("prompt_failed"))
		Expect(event.ProjectName).To(Equal("test-project"))
		Expect(event.PromptName).To(Equal("001-fail-notify.md"))
	})

	It("should call CommitOnly when no changelog", func() {
		promptPath := filepath.Join(promptsDir, "001-no-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.ContentReturns("# No changelog test", nil)
		manager.TitleReturns("No changelog test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return releaser.CommitOnlyCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify CommitAndRelease was NOT called
		Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))

		cancel()
	})

	It("should call CommitAndRelease with PatchBump when changelog exists", func() {
		promptPath := filepath.Join(promptsDir, "001-with-changelog.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.ContentReturns("# Fix bug", nil)
		manager.TitleReturns("Fix bug", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(true)
		releaser.GetNextVersionReturns("v0.1.1", nil)
		releaser.CommitAndReleaseReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return releaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify PatchBump was used
		_, bump := releaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.PatchBump))

		// Verify CommitOnly was NOT called
		Expect(releaser.CommitOnlyCallCount()).To(Equal(0))

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

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with title "Add new feature"
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Add new feature\n\nImplement new feature."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.ContentReturns("# Add new feature", nil)
		manager.TitleReturns("Add new feature", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(true)
		releaser.GetNextVersionReturns("v0.2.0", nil)
		releaser.CommitAndReleaseReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return releaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify MinorBump was used
		_, bump := releaser.CommitAndReleaseArgsForCall(0)
		Expect(bump).To(Equal(git.MinorBump))

		cancel()
	})

	It("should process multiple queued prompts sequentially", func() {
		promptPath1 := filepath.Join(promptsDir, "001-first.md")
		promptPath2 := filepath.Join(promptsDir, "002-second.md")

		// Return both prompts first, then just second, then none
		manager.ListQueuedReturnsOnCall(0, []prompt.Prompt{
			{Path: promptPath1, Status: prompt.ApprovedPromptStatus},
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
		}, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{
			{Path: promptPath2, Status: prompt.ApprovedPromptStatus},
		}, nil)
		manager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)

		manager.ContentReturns("# Test", nil)
		manager.TitleReturns("Test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for both to be processed
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 3*time.Second, 50*time.Millisecond).Should(Equal(2))

		cancel()
	})

	It("should sanitize container name", func() {
		promptPath := filepath.Join(promptsDir, "001-test@file#name.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.ContentReturns("# Test", nil)
		manager.TitleReturns("Test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify container name was sanitized
		_, _, _, containerName := executor.ExecuteArgsForCall(0)
		Expect(containerName).To(Equal("test-project-001-test-file-name"))

		cancel()
	})

	It("should auto-set status to queued when prompt has non-standard status", func() {
		promptPath := filepath.Join(promptsDir, "001-auto-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: ""}, // empty status triggers auto-set
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.SetStatusReturns(nil)
		manager.ContentReturns("# Auto status test", nil)
		manager.TitleReturns("Auto status test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for SetStatus to be called (auto-set to queued)
		Eventually(func() int {
			return manager.SetStatusCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify SetStatus was called with "approved"
		_, _, status := manager.SetStatusArgsForCall(0)
		Expect(status).To(Equal("approved"))

		cancel()
	})

	It("should skip prompt with invalid status", func() {
		promptPath := filepath.Join(promptsDir, "001-invalid-status.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ExecutingPromptStatus}, // Wrong status for execution
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.AllPreviousCompletedReturns(true)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should skip prompt when previous not completed", func() {
		promptPath := filepath.Join(promptsDir, "003-third.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.AllPreviousCompletedReturns(false) // Previous prompts not completed

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait briefly to ensure processing completes
		time.Sleep(300 * time.Millisecond)

		// Verify executor was NOT called
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		cancel()
	})

	It("should set version in frontmatter from version getter", func() {
		promptPath := filepath.Join(promptsDir, "001-version-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.ContentReturns("# Version test", nil)
		manager.TitleReturns("Version test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing (processor uses Load/PrepareForExecution now)
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Verify Load was called with correct path
		_, path := manager.LoadArgsForCall(0)
		Expect(path).To(Equal(promptPath))

		cancel()
	})

	It("should append completion report suffix to content before executor call", func() {
		promptPath := filepath.Join(promptsDir, "001-suffix-test.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		// Override Load to return PromptFile with specific content
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test prompt content\n\nContent for testing suffix."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.ContentReturns("# Test prompt content", nil)
		manager.TitleReturns("Suffix test", nil)
		manager.SetContainerReturns(nil)
		manager.SetVersionReturns(nil)
		manager.SetStatusReturns(nil)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		// Run processor in goroutine
		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for processing
		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Verify executor was called with content including suffix
		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
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

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Validation test\n\nContent for validation suffix test."),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.MoveToCompletedReturns(nil)
		manager.AllPreviousCompletedReturns(true)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(nil)

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"make precommit",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		Eventually(func() int {
			return executor.ExecuteCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
		Expect(promptContent).To(ContainSubstring(report.ValidationSuffix("make precommit")))

		cancel()
	})

	Describe("Worktree Workflow", func() {
		It("should add worktree, commit, push, create PR, and remove worktree", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Add new feature\n\nWorktree test content."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Worktree test", nil)
			manager.TitleReturns("Add new feature", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			// Mock worktree.Add to create the actual directory
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/123", nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree operations
			Expect(cloner.CloneCallCount()).To(Equal(1))
			_, _, worktreePath, branchName := cloner.CloneArgsForCall(0)
			Expect(worktreePath).To(ContainSubstring("dark-factory/test-project-001-worktree-test"))
			Expect(branchName).To(Equal("dark-factory/001-worktree-test"))

			// Verify push was called
			Expect(brancher.PushCallCount()).To(Equal(1))
			_, pushedBranch := brancher.PushArgsForCall(0)
			Expect(pushedBranch).To(Equal("dark-factory/001-worktree-test"))

			// Verify PR was created
			_, title, body := prCreator.CreateArgsForCall(0)
			Expect(title).To(Equal("Add new feature"))
			Expect(body).To(Equal("Automated by dark-factory"))

			// Verify worktree was removed
			Expect(cloner.RemoveCallCount()).To(BeNumerically(">=", 1))
			_, removedPath := cloner.RemoveArgsForCall(0)
			Expect(removedPath).To(ContainSubstring("dark-factory/test-project-001-worktree-test"))

			// Verify CommitOnly was called, not CommitAndRelease
			Expect(releaser.CommitOnlyCallCount()).To(Equal(1))
			Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))
			// HasChangelog is called once for changelog suffix check during content assembly
			Expect(releaser.HasChangelogCallCount()).To(Equal(1))

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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Log path test\n\nVerify log path is absolute."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Log path test", nil)
			manager.TitleReturns("Log path test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/test/repo/pull/99", nil)

			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				return nil
			}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Read captured args after Eventually confirms execution happened (safe - goroutine done writing)
			_, _, capturedLogFile, _ := executor.ExecuteArgsForCall(0)

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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Add feature\n\nWorktree auto-merge test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Worktree automerge test", nil)
			manager.TitleReturns("Add feature", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/test/repo/pull/2", nil)
			prMerger.WaitAndMergeReturns(nil)
			brancher.DefaultBranchReturns("master", nil)
			brancher.SwitchReturns(nil)
			brancher.PullReturns(nil)
			releaser.HasChangelogReturns(false)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				true, // autoMerge enabled
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to be called
			Eventually(func() int {
				return prMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree operations
			Expect(cloner.CloneCallCount()).To(Equal(1))
			Expect(prCreator.CreateCallCount()).To(Equal(1))
			Expect(cloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Verify WaitAndMerge was called with correct PR URL
			_, mergedURL := prMerger.WaitAndMergeArgsForCall(0)
			Expect(mergedURL).To(Equal("https://github.com/test/repo/pull/2"))

			// Verify post-merge actions
			Eventually(func() int {
				return brancher.DefaultBranchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))
			Eventually(func() int {
				return brancher.PullCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})

		It("should clean up worktree even on execution failure", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nFail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Fail test", nil)
			manager.TitleReturns("Test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			// Mock worktree.Add to create the actual directory
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			executor.ExecuteReturns(stderrors.New("execution failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for worktree to be cleaned up (via defer)
			Eventually(func() int {
				return cloner.RemoveCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify worktree was added
			Expect(cloner.CloneCallCount()).To(Equal(1))

			// Verify worktree was removed despite failure
			Expect(cloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			cancel()
		})

		It("should log warning but not fail when worktree removal fails", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-remove-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nRemove fail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Test", nil)
			manager.TitleReturns("Test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			// Mock worktree.Add to create the actual directory
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			// Mock worktree.Remove to return an error but don't actually remove
			cloner.RemoveReturns(stderrors.New("worktree removal failed"))
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/123", nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing - should complete despite remove failure
			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify worktree removal was attempted
			Expect(cloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Verify PR was created successfully despite cleanup failure
			Expect(prCreator.CreateCallCount()).To(Equal(1))

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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n\nWorktree merge fail test."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.ContentReturns("# Test", nil)
			manager.TitleReturns("Test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/test/repo/pull/3", nil)
			prMerger.WaitAndMergeReturns(stderrors.New("PR has conflicts"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				true, // autoMerge enabled
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to be called
			Eventually(func() int {
				return prMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Worktree was added and removed before merge attempt
			Expect(cloner.CloneCallCount()).To(Equal(1))
			Expect(cloner.RemoveCallCount()).To(BeNumerically(">=", 1))

			// Load should be called to mark prompt as failed
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})

		It("should handle worktree add error", func() {
			promptPath := filepath.Join(promptsDir, "001-worktree-add-error.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.ContentReturns("# Worktree add error test", nil)
			manager.TitleReturns("Test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			cloner.CloneReturns(stderrors.New("clone failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for Load to be called (marks prompt as failed)
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify executor was NOT called
			Expect(executor.ExecuteCallCount()).To(Equal(0))

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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
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
			manager.ContentReturns("# Summary test", nil)
			manager.TitleReturns("Summary test", nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report containing summary
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Added feature successfully","blockers":[]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			// Mock MoveToCompleted to verify frontmatter has summary
			manager.MoveToCompletedStub = func(ctx context.Context, path string) error {
				// Verify summary was set before moving
				Expect(savedPromptFile.Frontmatter.Summary).To(Equal("Added feature successfully"))
				return nil
			}

			p := processor.NewProcessor(
				promptsDir,
				completedDir,
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return manager.MoveToCompletedCallCount()
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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.ContentReturns("# Report success test", nil)
			manager.TitleReturns("Report success test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with success report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

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
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.ContentReturns("# Report failed test", nil)
			manager.TitleReturns("Report failed test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with failed report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

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
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.ContentReturns("# Report partial test", nil)
			manager.TitleReturns("Report partial test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with partial report
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

			cancel()
		})

		It("should fire prompt_partial notification when report status is partial", func() {
			promptPath := filepath.Join(promptsDir, "001-report-partial-notify.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log-notify")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.ContentReturns("# Report partial notify test", nil)
			manager.TitleReturns("Report partial notify test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
				logContent := `dark-factory: executing prompt
some output

<!-- DARK-FACTORY-REPORT
{"status":"partial","summary":"Half done","blockers":["lint fails"]}
DARK-FACTORY-REPORT -->
`
				return os.WriteFile(logFile, []byte(logContent), 0600)
			}

			fakeNotifier := &mocks.Notifier{}

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				fakeNotifier,
			)

			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return fakeNotifier.NotifyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))
			cancel()

			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("prompt_partial"))
			Expect(event.ProjectName).To(Equal("test-project"))
			Expect(event.PromptName).To(Equal("001-report-partial-notify.md"))
		})

		It("should continue when no report found (backwards compatible)", func() {
			promptPath := filepath.Join(promptsDir, "001-no-report.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			logDir := filepath.Join(tempDir, "log")
			err := os.MkdirAll(logDir, 0750)
			Expect(err).NotTo(HaveOccurred())

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.ContentReturns("# No report test", nil)
			manager.TitleReturns("No report test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log WITHOUT report (old-style prompt)
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

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
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.ContentReturns("# Verification override test", nil)
			manager.TitleReturns("Verification override test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)

			// Mock executor writes log with success report but non-zero verification exit code
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor — should not return error (continues after failure)
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for processing to complete
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify moved to completed was NOT called (status was overridden to partial)
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

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

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.ContentReturns("# Malformed report test", nil)
			manager.TitleReturns("Malformed report test", nil)
			manager.SetContainerReturns(nil)
			manager.SetVersionReturns(nil)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Mock executor writes log with malformed JSON
			executor.ExecuteStub = func(_ context.Context, _ string, logFile string, _ string) error {
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			// Run processor in goroutine
			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing - should continue despite malformed JSON
			Eventually(func() int {
				return releaser.CommitOnlyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify moved to completed
			Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

			cancel()
		})
	})

	Describe("Git sync before execution", func() {
		It("should call Fetch and MergeOriginDefault before processing prompt", func() {
			promptPath := filepath.Join(promptsDir, "001-sync-test.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for processing
			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify Fetch and MergeOriginDefault were called
			Expect(brancher.FetchCallCount()).To(Equal(1))
			Expect(brancher.MergeOriginDefaultCallCount()).To(Equal(1))

			cancel()
		})

		It("should fail prompt if Fetch fails", func() {
			promptPath := filepath.Join(promptsDir, "001-fetch-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			brancher.FetchReturns(stderrors.New("fetch failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for Fetch to be called
			Eventually(func() int {
				return brancher.FetchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Executor should NOT be called since Fetch failed
			Consistently(func() int {
				return executor.ExecuteCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Load should be called to mark as failed
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			cancel()
		})

		It("should fail prompt if MergeOriginDefault fails", func() {
			promptPath := filepath.Join(promptsDir, "001-merge-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(stderrors.New("merge conflict"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for MergeOriginDefault to be called
			Eventually(func() int {
				return brancher.MergeOriginDefaultCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Executor should NOT be called since MergeOriginDefault failed
			Consistently(func() int {
				return executor.ExecuteCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Load should be called to mark as failed
			Eventually(func() int {
				return manager.LoadCallCount()
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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Commit error test\n\nContent."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(stderrors.New("commit failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for failure to be handled (Load called to mark as failed)
			Eventually(func() int {
				return manager.LoadCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

			// Push and PR creation must NOT be called
			Expect(brancher.PushCallCount()).To(Equal(0))
			Expect(prCreator.CreateCallCount()).To(Equal(0))

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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, nil, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Push error test\n\nContent."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.PushReturns(stderrors.New("push failed"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for push to be attempted
			Eventually(func() int {
				return brancher.PushCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// PR creation must NOT be called
			Expect(prCreator.CreateCallCount()).To(Equal(0))

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
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, queued, nil)
			manager.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for at least 3 ListQueued calls (two skips + empty)
			Eventually(func() int {
				return manager.ListQueuedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 3))

			// Executor was never called — prompt was skipped both times
			Expect(executor.ExecuteCallCount()).To(Equal(0))

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
			manager.ListQueuedStub = func(_ context.Context) ([]prompt.Prompt, error) {
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
			manager.AllPreviousCompletedReturns(true)
			// Fail fast after shouldSkipPrompt passes to keep test simple
			brancher.FetchReturns(stderrors.New("fetch failed after retry"))

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Fetch is called only when shouldSkipPrompt returns false (i.e., prompt was retried)
			Eventually(func() int {
				return brancher.FetchCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})
	})

	It("should return error when CommitAndRelease fails in direct workflow", func() {
		promptPath := filepath.Join(promptsDir, "001-commitrelease-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(true)
		releaser.GetNextVersionReturns("v0.1.1", nil)
		releaser.CommitAndReleaseReturns(stderrors.New("commit and release failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Wait for CommitAndRelease to be called
		Eventually(func() int {
			return releaser.CommitAndReleaseCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load should be called to mark prompt as failed
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when MoveToCompleted fails for empty prompt", func() {
		promptPath := filepath.Join(promptsDir, "001-empty-move-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		// Empty body triggers ErrEmptyPrompt from pf.Content()
		manager.LoadReturns(
			prompt.NewPromptFile(
				promptPath,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte(""),
				libtime.NewCurrentDateTime(),
			),
			nil,
		)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(stderrors.New("move failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// MoveToCompleted was attempted
		Eventually(func() int {
			return manager.MoveToCompletedCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Executor was NOT called (prompt was empty)
		Expect(executor.ExecuteCallCount()).To(Equal(0))

		// Load should be called to mark prompt as failed (MoveToCompleted returned error)
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should keep running when ListQueued fails during startup scan (daemon mode)", func() {
		// Daemon mode swallows processExistingQueued errors — including ListQueued failures.
		// Process must continue running until the context is cancelled.
		manager.ListQueuedReturns(nil, stderrors.New("list queued failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- p.Process(ctx)
		}()

		// Let it run the startup scan
		time.Sleep(200 * time.Millisecond)

		// Daemon must still be running — cancel and verify clean shutdown
		cancel()
		select {
		case err := <-errCh:
			Expect(err).NotTo(HaveOccurred())
		case <-time.After(2 * time.Second):
			Fail("processor did not stop within timeout")
		}
		// Executor was never called
		Expect(executor.ExecuteCallCount()).To(Equal(0))
	})

	It("should return error when CommitCompletedFile fails", func() {
		promptPath := filepath.Join(promptsDir, "001-commitfile-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(stderrors.New("commit completed file failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Load called to mark as failed
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		// Executor was called but CommitCompletedFile failed
		Expect(executor.ExecuteCallCount()).To(Equal(1))

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

		cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		cloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.CommitOnlyReturns(nil)
		brancher.PushReturns(nil)
		prCreator.CreateReturns("https://github.com/test/pull/1", nil)
		manager.SetPRURLReturns(nil)
		prMerger.WaitAndMergeReturns(nil)
		brancher.DefaultBranchReturns("main", nil)
		// Switch fails after DefaultBranch succeeds
		brancher.SwitchReturns(stderrors.New("switch to default branch failed"))

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
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			true,
			true,
			brancher,
			prCreator,
			cloner,
			prMerger,
			true, // autoMerge enabled
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// DefaultBranch and Switch are called
		Eventually(func() int {
			return brancher.DefaultBranchCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return manager.LoadCallCount()
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

		cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		cloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.CommitOnlyReturns(nil)
		brancher.PushReturns(nil)
		prCreator.CreateReturns("https://github.com/test/pull/1", nil)
		manager.SetPRURLReturns(nil)
		prMerger.WaitAndMergeReturns(nil)
		brancher.DefaultBranchReturns("main", nil)
		brancher.SwitchReturns(nil)
		// Pull fails after Switch succeeds
		brancher.PullReturns(stderrors.New("pull failed"))

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
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			true,
			true,
			brancher,
			prCreator,
			cloner,
			prMerger,
			true, // autoMerge enabled
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// Pull is called and fails
		Eventually(func() int {
			return brancher.PullCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return manager.LoadCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

		cancel()
	})

	It("should return error when CommitOnly fails in direct workflow (no changelog)", func() {
		promptPath := filepath.Join(promptsDir, "001-commitonly-nochangelog-error.md")
		queued := []prompt.Prompt{
			{Path: promptPath, Status: prompt.ApprovedPromptStatus},
		}

		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.HasChangelogReturns(false)
		releaser.CommitOnlyReturns(stderrors.New("commit only failed"))

		p := processor.NewProcessor(
			promptsDir,
			filepath.Join(promptsDir, "completed"),
			filepath.Join(promptsDir, "log"),
			"test-project",
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			false,
			false,
			brancher,
			prCreator,
			cloner,
			prMerger,
			false,
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// CommitOnly was called and failed
		Eventually(func() int {
			return releaser.CommitOnlyCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return manager.LoadCallCount()
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

		cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
			return os.MkdirAll(destDir, 0750)
		}
		cloner.RemoveStub = func(_ context.Context, path string) error {
			return os.RemoveAll(path)
		}
		manager.ListQueuedReturnsOnCall(0, queued, nil)
		manager.ListQueuedReturnsOnCall(1, nil, nil)
		manager.AllPreviousCompletedReturns(true)
		manager.MoveToCompletedReturns(nil)
		executor.ExecuteReturns(nil)
		releaser.CommitCompletedFileReturns(nil)
		releaser.CommitOnlyReturns(nil)
		brancher.PushReturns(nil)
		prCreator.CreateReturns("https://github.com/test/pull/1", nil)
		manager.SetPRURLReturns(nil)
		prMerger.WaitAndMergeReturns(nil)
		// DefaultBranch fails after successful merge
		brancher.DefaultBranchReturns("", stderrors.New("cannot determine default branch"))

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
			executor,
			manager,
			releaser,
			versionGet,
			ready,
			true,
			true,
			brancher,
			prCreator,
			cloner,
			prMerger,
			true, // autoMerge enabled
			false,
			false,
			autoCompleter,
			specLister,
			"",
			false,
			notifier.NewMultiNotifier(),
		)

		go func() {
			_ = p.Process(ctx)
		}()

		// WaitAndMerge is called
		Eventually(func() int {
			return prMerger.WaitAndMergeCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// DefaultBranch is called and fails → prompt marked as failed
		Eventually(func() int {
			return brancher.DefaultBranchCallCount()
		}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

		// Load called to mark as failed
		Eventually(func() int {
			return manager.LoadCallCount()
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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(nil)
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			manager.MoveToCompletedReturns(nil)
			manager.SetPRURLReturns(nil)

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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false, // autoMerge disabled
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for PR to be created
			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// WaitAndMerge should NOT be called
			Consistently(func() int {
				return prMerger.WaitAndMergeCallCount()
			}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

			// Worktree should be removed after PR creation
			Eventually(func() int {
				return cloner.RemoveCallCount()
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

				cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				cloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.CommitOnlyReturns(nil)
				brancher.FetchReturns(nil)
				brancher.MergeOriginDefaultReturns(nil)
				brancher.PushReturns(nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.SwitchReturns(nil)
				brancher.PullReturns(nil)
				prCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
				prMerger.WaitAndMergeReturns(nil)
				manager.MoveToCompletedReturns(nil)
				manager.SetPRURLReturns(nil)
				releaser.HasChangelogReturns(false)

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
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					true,
					true,
					brancher,
					prCreator,
					cloner,
					prMerger,
					true, // autoMerge enabled
					false,
					false,
					autoCompleter,
					specLister,
					"",
					false,
					notifier.NewMultiNotifier(),
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for WaitAndMerge to be called
				Eventually(func() int {
					return prMerger.WaitAndMergeCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// Verify DefaultBranch was called
				Eventually(func() int {
					return brancher.DefaultBranchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// Verify Switch to default branch was called
				Eventually(func() int {
					return brancher.SwitchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				// Verify Pull was called
				Eventually(func() int {
					return brancher.PullCallCount()
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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(nil)
			brancher.PushReturns(nil)
			brancher.DefaultBranchReturns("main", nil)
			brancher.SwitchReturns(nil)
			brancher.PullReturns(nil)
			prCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			prMerger.WaitAndMergeReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.SetPRURLReturns(nil)
			releaser.HasChangelogReturns(true)
			releaser.GetNextVersionReturns("v0.0.2", nil)
			releaser.CommitAndReleaseReturns(nil)

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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				true, // autoMerge enabled
				true, // autoRelease enabled
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for CommitAndRelease to be called
			Eventually(func() int {
				return releaser.CommitAndReleaseCallCount()
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

				cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				cloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.CommitOnlyReturns(nil)
				brancher.FetchReturns(nil)
				brancher.MergeOriginDefaultReturns(nil)
				brancher.PushReturns(nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.SwitchReturns(nil)
				brancher.PullReturns(nil)
				prCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
				prMerger.WaitAndMergeReturns(nil)
				manager.MoveToCompletedReturns(nil)
				manager.SetPRURLReturns(nil)
				releaser.HasChangelogReturns(false) // No changelog

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
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					true,
					true,
					brancher,
					prCreator,
					cloner,
					prMerger,
					true, // autoMerge enabled
					true, // autoRelease enabled
					false,
					autoCompleter,
					specLister,
					"",
					false,
					notifier.NewMultiNotifier(),
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for WaitAndMerge to complete
				Eventually(func() int {
					return prMerger.WaitAndMergeCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// CommitAndRelease should NOT be called
				Consistently(func() int {
					return releaser.CommitAndReleaseCallCount()
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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(nil)
			brancher.PushReturns(nil)
			brancher.DefaultBranchReturns("main", nil)
			brancher.SwitchReturns(nil)
			brancher.PullReturns(nil)
			prCreator.CreateReturns("https://github.com/test/test/pull/1", nil)
			prMerger.WaitAndMergeReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.SetPRURLReturns(nil)
			releaser.HasChangelogReturns(true) // Changelog exists but autoRelease is false

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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				true,  // autoMerge enabled
				false, // autoRelease disabled
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for WaitAndMerge to complete
			Eventually(func() int {
				return prMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// CommitAndRelease should NOT be called
			Consistently(func() int {
				return releaser.CommitAndReleaseCallCount()
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

				cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
					return os.MkdirAll(destDir, 0750)
				}
				cloner.RemoveStub = func(_ context.Context, path string) error {
					return os.RemoveAll(path)
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(nil)
				releaser.CommitOnlyReturns(nil)
				brancher.FetchReturns(nil)
				brancher.MergeOriginDefaultReturns(nil)
				brancher.PushReturns(nil)
				prCreator.CreateReturns("https://github.com/test/test/pull/42", nil)
				manager.SetStatusReturns(nil)
				manager.SetPRURLReturns(nil)

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
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					true,
					true,
					brancher,
					prCreator,
					cloner,
					prMerger,
					false, // autoMerge disabled
					false,
					true, // autoReview enabled
					autoCompleter,
					specLister,
					"",
					false,
					notifier.NewMultiNotifier(),
				)

				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for PR to be created
				Eventually(func() int {
					return prCreator.CreateCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// SetStatus should be called with in_review
				Eventually(func() string {
					if manager.SetStatusCallCount() == 0 {
						return ""
					}
					_, _, status := manager.SetStatusArgsForCall(
						manager.SetStatusCallCount() - 1,
					)
					return status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(string(prompt.InReviewPromptStatus)))

				// MoveToCompleted should NOT be called
				Consistently(func() int {
					return manager.MoveToCompletedCallCount()
				}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(0))

				// WaitAndMerge should NOT be called
				Consistently(func() int {
					return prMerger.WaitAndMergeCallCount()
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

			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			brancher.FetchReturns(nil)
			brancher.MergeOriginDefaultReturns(nil)
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/test/test/pull/43", nil)
			manager.MoveToCompletedReturns(nil)
			manager.SetPRURLReturns(nil)

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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true,
				true,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false, // autoMerge disabled
				false,
				false, // autoReview disabled
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			go func() {
				_ = p.Process(ctx)
			}()

			// Wait for PR to be created
			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// MoveToCompleted should be called (normal flow)
			Eventually(func() int {
				return manager.MoveToCompletedCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// SetStatus should NOT be called with in_review
			Consistently(func() bool {
				for i := 0; i < manager.SetStatusCallCount(); i++ {
					_, _, status := manager.SetStatusArgsForCall(i)
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
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
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
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(nil)
				releaser.HasChangelogReturns(false)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					false,
					false,
					brancher,
					prCreator,
					cloner,
					prMerger,
					false,
					false,
					false,
					autoCompleter,
					specLister,
					"",
					true, // verificationGate enabled
					notifier.NewMultiNotifier(),
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				Eventually(func() int {
					return executor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// MoveToCompleted must NOT be called — git ops are deferred
				Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

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
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(stderrors.New("execution failed"))

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					false,
					false,
					brancher,
					prCreator,
					cloner,
					prMerger,
					false,
					false,
					false,
					autoCompleter,
					specLister,
					"",
					true, // verificationGate enabled
					notifier.NewMultiNotifier(),
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				Eventually(func() int {
					return executor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// MoveToCompleted must NOT be called
				Expect(manager.MoveToCompletedCallCount()).To(Equal(0))

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
						executor,
						manager,
						releaser,
						versionGet,
						ready,
						false,
						false,
						brancher,
						prCreator,
						cloner,
						prMerger,
						false,
						false,
						false,
						autoCompleter,
						specLister,
						"",
						false,
						notifier.NewMultiNotifier(),
					)

					errCh := make(chan error, 1)
					go func() {
						errCh <- p.Process(ctx)
					}()

					// Give the processor time to run the initial scan
					time.Sleep(200 * time.Millisecond)

					// ListQueued must NOT be called — queue is blocked before the loop
					Expect(manager.ListQueuedCallCount()).To(Equal(0))
					// Executor must NOT be called
					Expect(executor.ExecuteCallCount()).To(Equal(0))

					cancel()
					<-errCh
				},
			)
		})

		Context("hasPendingVerification false, queue proceeds normally", func() {
			It("calls ListQueued when no pending_verification prompt exists", func() {
				manager.ListQueuedReturns([]prompt.Prompt{}, nil)

				p := processor.NewProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					false,
					false,
					brancher,
					prCreator,
					cloner,
					prMerger,
					false,
					false,
					false,
					autoCompleter,
					specLister,
					"",
					true, // gate enabled but no pending file
					notifier.NewMultiNotifier(),
				)

				errCh := make(chan error, 1)
				go func() {
					errCh <- p.Process(ctx)
				}()

				// ListQueued should be called (no pending file to block)
				Eventually(func() int {
					return manager.ListQueuedCallCount()
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
			realLister := spec.NewLister(libtime.NewCurrentDateTime(), specDir)

			manager.ListQueuedReturns([]prompt.Prompt{}, nil)
			autoCompleter.CheckAndCompleteReturns(nil)

			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				realLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)

			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for CheckAndComplete to be called with the spec name
			Eventually(func() int {
				return autoCompleter.CheckAndCompleteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, specID := autoCompleter.CheckAndCompleteArgsForCall(0)
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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)
		}

		It("ProcessQueue logs 'no queued prompts' once when queue is empty", func() {
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).NotTo(HaveOccurred())

			output := logBuf.String()
			Expect(strings.Count(output, "no queued prompts")).To(Equal(1))
		})

		It("ProcessQueue does not log 'no queued prompts, exiting'", func() {
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(logBuf.String()).NotTo(ContainSubstring("no queued prompts, exiting"))
		})

		It("daemon Process logs 'waiting for changes' once after startup scan", func() {
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

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
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

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
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				pr,
				worktree,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)
		}

		It("worktree=false, branch='': no branch switch called", func() {
			promptPath := filepath.Join(promptsDir, "001-no-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			p := newProcWithWorktree(false, false)
			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			Expect(brancher.IsCleanCallCount()).To(Equal(0))
			Expect(brancher.SwitchCallCount()).To(Equal(0))
			Expect(brancher.CreateAndSwitchCallCount()).To(Equal(0))

			cancel()
		})

		Context("when pr=false and worktree=false and prompt has a branch field", func() {
			It("ignores the branch field and does not attempt to switch branches", func() {
				promptPath := filepath.Join(promptsDir, "001-direct-with-branch.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "some-feature-branch"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				manager.MoveToCompletedReturns(nil)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.HasChangelogReturns(false)
				releaser.CommitOnlyReturns(nil)
				autoCompleter.CheckAndCompleteReturns(nil)

				p := newProcWithWorktree(false, false)
				err := p.ProcessQueue(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(brancher.IsCleanCallCount()).To(Equal(0))
				Expect(brancher.SwitchCallCount()).To(Equal(0))
				Expect(brancher.CreateAndSwitchCallCount()).To(Equal(0))
			})
		})

		It(
			"pr=true, worktree=false, branch set, clean, branch exists remotely: Switch called not CreateAndSwitch",
			func() {
				promptPath := filepath.Join(promptsDir, "001-branch-exists.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/test"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				manager.MoveToCompletedReturns(nil)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.HasChangelogReturns(false)
				releaser.CommitOnlyReturns(nil)

				brancher.IsCleanReturns(true, nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.FetchAndVerifyBranchReturns(nil) // branch exists remotely
				brancher.SwitchReturns(nil)

				p := newProcWithWorktree(true, false)
				go func() {
					_ = p.Process(ctx)
				}()

				Eventually(func() int {
					return executor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(brancher.IsCleanCallCount()).To(Equal(1))
				Expect(brancher.FetchAndVerifyBranchCallCount()).To(Equal(1))
				_, branchArg := brancher.FetchAndVerifyBranchArgsForCall(0)
				Expect(branchArg).To(Equal("dark-factory/test"))
				// Switch called: once to switch to branch, once to restore default
				Expect(brancher.SwitchCallCount()).To(BeNumerically(">=", 1))
				_, switchArg := brancher.SwitchArgsForCall(0)
				Expect(switchArg).To(Equal("dark-factory/test"))
				Expect(brancher.CreateAndSwitchCallCount()).To(Equal(0))

				cancel()
			},
		)

		It(
			"pr=true, worktree=false, branch set, clean, branch not on remote: CreateAndSwitch called",
			func() {
				promptPath := filepath.Join(promptsDir, "001-branch-new.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/test"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				manager.MoveToCompletedReturns(nil)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.HasChangelogReturns(false)
				releaser.CommitOnlyReturns(nil)

				brancher.IsCleanReturns(true, nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.FetchAndVerifyBranchReturns(
					stderrors.New("branch not found"),
				) // branch does not exist
				brancher.CreateAndSwitchReturns(nil)
				brancher.SwitchReturns(nil)

				p := newProcWithWorktree(true, false)
				go func() {
					_ = p.Process(ctx)
				}()

				Eventually(func() int {
					return executor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(brancher.IsCleanCallCount()).To(Equal(1))
				Expect(brancher.FetchAndVerifyBranchCallCount()).To(Equal(1))
				Expect(brancher.CreateAndSwitchCallCount()).To(Equal(1))
				_, createArg := brancher.CreateAndSwitchArgsForCall(0)
				Expect(createArg).To(Equal("dark-factory/test"))

				cancel()
			},
		)

		It(
			"pr=true, worktree=false, branch set, dirty tree: returns error, no branch operation",
			func() {
				promptPath := filepath.Join(promptsDir, "001-dirty.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/test"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)

				brancher.IsCleanReturns(false, nil) // dirty working tree

				p := newProcWithWorktree(true, false)
				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for the IsClean call to happen (prompt fails)
				Eventually(func() int {
					return brancher.IsCleanCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(executor.ExecuteCallCount()).To(Equal(0))
				Expect(brancher.SwitchCallCount()).To(Equal(0))
				Expect(brancher.CreateAndSwitchCallCount()).To(Equal(0))

				cancel()
			},
		)

		It("worktree=true, branch set: uses clone workflow, not in-place", func() {
			promptPath := filepath.Join(promptsDir, "001-clone-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "dark-factory/test"), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/456", nil)

			p := newProcWithWorktree(true, true)
			go func() {
				_ = p.Process(ctx)
			}()

			Eventually(func() int {
				return cloner.CloneCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// In-place branch switching should NOT have been called
			Expect(brancher.IsCleanCallCount()).To(Equal(0))

			cancel()
		})

		It(
			"pr=true, worktree=false: restores default branch after direct workflow with in-place branch",
			func() {
				promptPath := filepath.Join(promptsDir, "001-restore.md")
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "dark-factory/restore-test"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				manager.MoveToCompletedReturns(nil)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.HasChangelogReturns(false)
				releaser.CommitOnlyReturns(nil)

				brancher.IsCleanReturns(true, nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
				brancher.CreateAndSwitchReturns(nil)
				brancher.SwitchReturns(nil)

				p := newProcWithWorktree(true, false)
				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for execution to complete
				Eventually(func() int {
					return releaser.CommitOnlyCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// Switch should be called: once to restore default branch
				Expect(brancher.SwitchCallCount()).To(BeNumerically(">=", 1))
				// Find the restore call (last Switch call should be to "main")
				lastIdx := brancher.SwitchCallCount() - 1
				_, lastSwitchArg := brancher.SwitchArgsForCall(lastIdx)
				Expect(lastSwitchArg).To(Equal("main"))

				cancel()
			},
		)
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
					executor,
					manager,
					releaser,
					versionGet,
					ready,
					true,  // pr=true enables in-place branch switching
					false, // worktree=false
					brancher,
					prCreator,
					cloner,
					prMerger,
					false,
					false,
					false,
					autoCompleter,
					specLister,
					"",
					false,
					notifier.NewMultiNotifier(),
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

				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createBranchPromptFile(path, "feature/my-branch"), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				manager.MoveToCompletedReturns(nil)
				// More prompts on branch — skip merge
				manager.HasQueuedPromptsOnBranchReturns(true, nil)
				executor.ExecuteReturns(nil)
				releaser.CommitCompletedFileReturns(nil)
				releaser.HasChangelogReturns(true) // changelog exists but should NOT release
				releaser.CommitOnlyReturns(nil)

				brancher.IsCleanReturns(true, nil)
				brancher.DefaultBranchReturns("main", nil)
				brancher.FetchAndVerifyBranchReturns(stderrors.New("not found"))
				brancher.CreateAndSwitchReturns(nil)
				brancher.SwitchReturns(nil)

				p := newProcDirect()
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return releaser.CommitOnlyCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// CommitAndRelease must NOT be called
				Expect(releaser.CommitAndReleaseCallCount()).To(Equal(0))

				cancel()
			},
		)

		It("default branch with changelog: CommitAndRelease called (unchanged behavior)", func() {
			promptPath := filepath.Join(promptsDir, "001-default-changelog.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(true)
			releaser.GetNextVersionReturns("v0.1.1", nil)
			releaser.CommitAndReleaseReturns(nil)

			p := newProcDirect()
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return releaser.CommitAndReleaseCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// CommitOnly must NOT be called
			Expect(releaser.CommitOnlyCallCount()).To(Equal(0))

			cancel()
		})

		It("pr=true: handleBranchCompletion NOT called even when feature branch completes", func() {
			promptPath := filepath.Join(promptsDir, "001-pr-branch.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			// pr=true with worktree=true uses clone workflow
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createBranchPromptFile(path, "feature/pr-mode"), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/789", nil)

			// pr=true, worktree=true
			p := processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true, // pr=true
				true, // worktree=true
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// handleBranchCompletion must NOT be called (pr=true uses clone workflow)
			Expect(manager.HasQueuedPromptsOnBranchCallCount()).To(Equal(0))
			Expect(brancher.MergeToDefaultCallCount()).To(Equal(0))

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
			cloner.CloneStub = func(_ context.Context, _, destDir string, _ string) error {
				return os.MkdirAll(destDir, 0750)
			}
			cloner.RemoveStub = func(_ context.Context, path string) error {
				return os.RemoveAll(path)
			}
			brancher.PushReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.CommitOnlyReturns(nil)
		}

		newProcWorktree := func(autoMerge bool) processor.Processor {
			return processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				true, // pr=true
				true, // worktree=true
				brancher,
				prCreator,
				cloner,
				prMerger,
				autoMerge,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)
		}

		It("FindOpenPR returns existing URL: Create NOT called, existing URL used", func() {
			promptPath := filepath.Join(promptsDir, "001-idempotent.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/idempotent", ""), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			// Existing PR found
			prCreator.FindOpenPRReturns("https://github.com/user/repo/pull/42", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.FindOpenPRCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Create must NOT be called when PR already exists
			Expect(prCreator.CreateCallCount()).To(Equal(0))

			cancel()
		})

		It("FindOpenPR returns empty: Create called with buildPRBody result", func() {
			promptPath := filepath.Join(promptsDir, "001-create-pr.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/new-pr", ""), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			// No existing PR
			prCreator.FindOpenPRReturns("", nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/99", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Verify body is the default (no issue)
			_, _, body := prCreator.CreateArgsForCall(0)
			Expect(body).To(Equal("Automated by dark-factory"))

			cancel()
		})

		It("pf.Issue() non-empty: PR body contains issue reference", func() {
			promptPath := filepath.Join(promptsDir, "001-with-issue.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/issue-ref", "BRO-42"), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			prCreator.FindOpenPRReturns("", nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/100", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, _, body := prCreator.CreateArgsForCall(0)
			Expect(body).To(ContainSubstring("Issue: BRO-42"))
			Expect(body).To(Equal("Automated by dark-factory\n\nIssue: BRO-42"))

			cancel()
		})

		It("pf.Issue() empty: PR body is default without issue line", func() {
			promptPath := filepath.Join(promptsDir, "001-no-issue.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/no-issue", ""), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			prCreator.FindOpenPRReturns("", nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/101", nil)

			p := newProcWorktree(false)
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, _, body := prCreator.CreateArgsForCall(0)
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
				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return createIssuePromptFile(path, "feature/multi", ""), nil
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				setupCloneMocks()
				prCreator.FindOpenPRReturns("https://github.com/user/repo/pull/50", nil)
				// More prompts on branch — defer merge
				manager.HasQueuedPromptsOnBranchReturns(true, nil)

				p := newProcWorktree(true) // autoMerge=true
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return manager.HasQueuedPromptsOnBranchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				// WaitAndMerge must NOT be called
				Expect(prMerger.WaitAndMergeCallCount()).To(Equal(0))
				// Prompt must be moved to completed
				Expect(manager.MoveToCompletedCallCount()).To(Equal(1))

				cancel()
			},
		)

		It("autoMerge=true, HasQueuedPromptsOnBranch=false: WaitAndMerge called", func() {
			promptPath := filepath.Join(promptsDir, "001-last-prompt.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/last", ""), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			prCreator.FindOpenPRReturns("https://github.com/user/repo/pull/51", nil)
			// Last prompt on branch — trigger merge
			manager.HasQueuedPromptsOnBranchReturns(false, nil)
			prMerger.WaitAndMergeReturns(nil)
			brancher.DefaultBranchReturns("main", nil)
			brancher.SwitchReturns(nil)
			brancher.PullReturns(nil)
			releaser.HasChangelogReturns(false)

			p := newProcWorktree(true) // autoMerge=true
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prMerger.WaitAndMergeCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			cancel()
		})

		It("autoMerge=false: PR created but WaitAndMerge NOT called", func() {
			promptPath := filepath.Join(promptsDir, "001-no-merge.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return createIssuePromptFile(path, "feature/no-automerge", ""), nil
			}
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			setupCloneMocks()
			prCreator.FindOpenPRReturns("", nil)
			prCreator.CreateReturns("https://github.com/user/repo/pull/102", nil)

			p := newProcWorktree(false) // autoMerge=false
			go func() { _ = p.Process(ctx) }()

			Eventually(func() int {
				return prCreator.CreateCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// WaitAndMerge must NOT be called
			Expect(prMerger.WaitAndMergeCallCount()).To(Equal(0))

			cancel()
		})
	})

	Describe("stop-on-failure behavior", func() {
		newProc := func() processor.Processor {
			return processor.NewProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				ready,
				false,
				false,
				brancher,
				prCreator,
				cloner,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				false,
				notifier.NewMultiNotifier(),
			)
		}

		It("ProcessQueue returns non-nil error when a prompt fails", func() {
			promptPath := filepath.Join(promptsDir, "001-fail.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(stderrors.New("execution failed"))

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("ProcessQueue does not call ResetFailed on startup", func() {
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			err := p.ProcessQueue(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(manager.ResetFailedCallCount()).To(Equal(0))
		})

		It("Process does not call ResetFailed on startup", func() {
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)

			p := newProc()
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Let it run the startup scan
			time.Sleep(200 * time.Millisecond)
			cancel()

			select {
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(2 * time.Second):
				Fail("processor did not stop within timeout")
			}

			Expect(manager.ResetFailedCallCount()).To(Equal(0))
		})

		It("Process continues running after prompt failure (daemon mode)", func() {
			promptPath := filepath.Join(promptsDir, "001-fail-daemon.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			// First call returns the failing prompt; subsequent calls return empty
			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(stderrors.New("execution failed"))

			p := newProc()
			errCh := make(chan error, 1)
			go func() {
				errCh <- p.Process(ctx)
			}()

			// Wait for the executor to be called (prompt processed)
			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			// Daemon should still be running — cancel and verify clean nil return
			cancel()
			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
			case <-time.After(2 * time.Second):
				Fail("processor did not stop within timeout")
			}
		})
	})
})
