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
				false,
				config.WorkflowDirect,
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
				false,
				config.WorkflowDirect,
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
				false,
				config.WorkflowDirect,
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

})
