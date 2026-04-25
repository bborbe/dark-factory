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

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

var _ = Describe("Processor", func() {
	var (
		tempDir       string
		promptsDir    string
		wakeup        chan struct{}
		ctx           context.Context
		cancel        context.CancelFunc
		executor      *mocks.Executor
		manager       *mocks.ProcessorPromptManager
		releaser      *mocks.Releaser
		versionGet    *mocks.VersionGetter
		brancher      *mocks.Brancher
		prCreator     *mocks.PRCreator
		cloner        *mocks.Cloner
		worktreer     *mocks.Worktreer
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

		wakeup = make(chan struct{}, 10)
		ctx, cancel = context.WithCancel(context.Background())

		executor = &mocks.Executor{}
		manager = &mocks.ProcessorPromptManager{}
		releaser = &mocks.Releaser{}
		versionGet = &mocks.VersionGetter{}
		brancher = &mocks.Brancher{}
		brancher.CommitsAheadReturns(1, nil)
		prCreator = &mocks.PRCreator{}
		cloner = &mocks.Cloner{}
		worktreer = &mocks.Worktreer{}
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

	BeforeEach(func() {
		manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
			return newProcessorTestPromptFile(path, "# Test\n\nDefault test content"), nil
		}
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				logDir,
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				true, // autoMerge enabled
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				true, // autoMerge enabled
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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
			manager.SetStatusReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			cloner.CloneReturns(stderrors.New("clone failed"))

			p := newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				true,
				config.WorkflowClone,
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				false,
				false,
				false,
				autoCompleter,
				specLister,
				"",
				"",
				"",
				false,
				notifier.NewMultiNotifier(),
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				0,
				0,
				nil,
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
})
