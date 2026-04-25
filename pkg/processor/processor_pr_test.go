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
				true,                 // pr=true
				config.WorkflowClone, // workflow
				brancher,
				prCreator,
				cloner,
				worktreer,
				prMerger,
				autoMerge,
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
		}

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
			// ResetFailed is not in the narrow PromptManager interface; processor cannot call it
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

	Describe("additionalInstructions", func() {
		It("prepends additionalInstructions to prompt content before executor call", func() {
			promptPath := filepath.Join(promptsDir, "001-additional-instructions.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# My prompt\n\nContent here."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

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
				"Read /docs/guide.md before starting.",
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

			_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
			Expect(promptContent).To(HavePrefix("Read /docs/guide.md before starting.\n\n"))
			Expect(promptContent).To(ContainSubstring("# My prompt"))

			cancel()
		})

		It("does not prepend anything when additionalInstructions is empty", func() {
			promptPath := filepath.Join(promptsDir, "001-no-additional.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.LoadReturns(
				prompt.NewPromptFile(
					promptPath,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# My prompt\n\nContent here."),
					libtime.NewCurrentDateTime(),
				),
				nil,
			)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

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

			Eventually(func() int {
				return executor.ExecuteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, promptContent, _, _ := executor.ExecuteArgsForCall(0)
			Expect(promptContent).To(HavePrefix("# My prompt"))

			cancel()
		})
	})

})
