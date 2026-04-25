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
	"github.com/bborbe/dark-factory/pkg/processor"
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

	Describe("In-place branch switching", func() {
		createBranchPromptFile := func(path string, branch string) *prompt.PromptFile {
			return prompt.NewPromptFile(
				path,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus), Branch: branch},
				[]byte("# Test\n\nDefault test content"),
				libtime.NewCurrentDateTime(),
			)
		}

		newProcWithWorkflow := func(pr bool, workflow config.Workflow) processor.Processor {
			return newTestProcessor(
				promptsDir,
				filepath.Join(promptsDir, "completed"),
				filepath.Join(promptsDir, "log"),
				"test-project",
				executor,
				manager,
				releaser,
				versionGet,
				wakeup,
				pr,
				workflow,
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

			p := newProcWithWorkflow(false, config.WorkflowDirect)
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

				p := newProcWithWorkflow(false, config.WorkflowDirect)
				go func() { _ = p.Process(ctx) }()

				Eventually(func() int {
					return executor.ExecuteCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				Expect(brancher.IsCleanCallCount()).To(Equal(0))
				Expect(brancher.SwitchCallCount()).To(Equal(0))
				Expect(brancher.CreateAndSwitchCallCount()).To(Equal(0))
				cancel()
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

				p := newProcWithWorkflow(true, config.WorkflowBranch)
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

				p := newProcWithWorkflow(true, config.WorkflowBranch)
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

				p := newProcWithWorkflow(true, config.WorkflowBranch)
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

			p := newProcWithWorkflow(true, config.WorkflowClone)
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

				p := newProcWithWorkflow(true, config.WorkflowBranch)
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
				return newTestProcessor(
					promptsDir,
					filepath.Join(promptsDir, "completed"),
					filepath.Join(promptsDir, "log"),
					"test-project",
					executor,
					manager,
					releaser,
					versionGet,
					wakeup,
					true,                  // pr=true enables in-place branch switching
					config.WorkflowBranch, // workflow
					brancher,
					prCreator,
					cloner,
					worktreer,
					prMerger,
					false,
					true, // autoRelease=true
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
				true,                 // pr=true
				config.WorkflowClone, // workflow
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

})
