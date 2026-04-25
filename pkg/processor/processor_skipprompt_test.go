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

})
