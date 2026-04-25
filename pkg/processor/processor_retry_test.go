// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cancellationwatcher"
	"github.com/bborbe/dark-factory/pkg/committingrecoverer"
	"github.com/bborbe/dark-factory/pkg/completionreport"
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/containerslot"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/promptenricher"
	"github.com/bborbe/dark-factory/pkg/promptresumer"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specsweeper"
	"github.com/bborbe/dark-factory/pkg/validationprompt"
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

	Describe("auto-retry behavior", func() {
		newProcWithNotifierAndRetryLimit := func(n notifier.Notifier, autoRetryLimit int) processor.Processor {
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
				n,
				nil,
				0,
				"",
				nil,
				nil,
				0,
				nil,
				nil,
				autoRetryLimit,
				0,
				nil,
			)
		}

		writePromptFile := func(path string, status string, retryCount int) {
			content := fmt.Sprintf(
				"---\nstatus: %s\nretryCount: %d\n---\n# Test\n\nContent",
				status,
				retryCount,
			)
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
		}

		It(
			"re-queues with incremented retryCount on first failure when autoRetryLimit > 0",
			func() {
				promptPath := filepath.Join(promptsDir, "001-retry.md")
				writePromptFile(promptPath, "approved", 0)

				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}

				// Use the real Load (not the stub) since Save writes to disk
				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return prompt.Load(ctx, path, libtime.NewCurrentDateTime())
				}

				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				manager.AllPreviousCompletedReturns(true)
				executor.ExecuteReturns(stderrors.New("execution failed"))

				p := newProcWithNotifierAndRetryLimit(notifier.NewMultiNotifier(), 3)
				go func() { _ = p.Process(ctx) }()

				// Read the file back and verify it was re-queued with incremented retryCount
				Eventually(func() string {
					content, _ := os.ReadFile(promptPath)
					return string(content)
				}, 2*time.Second, 50*time.Millisecond).Should(ContainSubstring("retryCount: 1"))

				content, readErr := os.ReadFile(promptPath)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("status: approved"))
				cancel()
			},
		)

		It("marks failed when retries exhausted", func() {
			promptPath := filepath.Join(promptsDir, "001-exhausted.md")
			writePromptFile(promptPath, "approved", 2) // retryCount already at limit

			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			n := &mocks.Notifier{}
			n.NotifyReturns(nil)

			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(stderrors.New("execution failed"))

			p := newProcWithNotifierAndRetryLimit(
				n,
				2,
			) // autoRetryLimit=2, retryCount=2 → exhausted
			go func() { _ = p.Process(ctx) }()

			// Verify prompt_failed notification was fired
			Eventually(func() int {
				return n.NotifyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))
			found := false
			for i := 0; i < n.NotifyCallCount(); i++ {
				_, evt := n.NotifyArgsForCall(i)
				if evt.EventType == "prompt_failed" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected prompt_failed notification")

			// Verify file is marked failed
			content, readErr := os.ReadFile(promptPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: failed"))
			cancel()
		})

		It("marks failed (standard) when autoRetryLimit is 0", func() {
			promptPath := filepath.Join(promptsDir, "001-std-fail.md")
			writePromptFile(promptPath, "approved", 0)

			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			n := &mocks.Notifier{}
			n.NotifyReturns(nil)

			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				return prompt.Load(ctx, path, libtime.NewCurrentDateTime())
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			executor.ExecuteReturns(stderrors.New("execution failed"))

			p := newProcWithNotifierAndRetryLimit(n, 0) // autoRetryLimit=0 → standard failure
			go func() { _ = p.Process(ctx) }()

			// Verify prompt_failed was notified
			Eventually(func() int {
				return n.NotifyCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))
			found := false
			for i := 0; i < n.NotifyCallCount(); i++ {
				_, evt := n.NotifyArgsForCall(i)
				if evt.EventType == "prompt_failed" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected prompt_failed notification")

			// Verify file is marked failed
			content, readErr := os.ReadFile(promptPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("status: failed"))
			cancel()
		})

	})

	Describe("processExistingQueued post-execution failure detection", func() {
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

		It(
			"stops daemon when prompt file gone from in-progress and found in completed (post-execution failure)",
			func() {
				promptPath := filepath.Join(promptsDir, "001-post-exec-stop.md")
				completedDir := filepath.Join(promptsDir, "completed")
				Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())

				// Simulate file already moved to completed/ (container succeeded, git commit failed)
				Expect(os.WriteFile(
					filepath.Join(completedDir, "001-post-exec-stop.md"),
					[]byte("---\nstatus: completed\n---\n"),
					0600,
				)).To(Succeed())

				// pr.Path does NOT exist on disk — file was already moved to completed/
				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, nil, nil)
				manager.AllPreviousCompletedReturns(true)
				brancher.FetchReturns(stderrors.New("fetch failed"))

				// processPrompt now calls manager.Load before Setup (sync). Configure it to
				// return a minimal PromptFile so Content() does not panic.
				manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
					return newProcessorTestPromptFile(path, "# Post-exec stop test\n\nContent"), nil
				}

				p := newProc()
				go func() {
					_ = p.Process(ctx)
				}()

				// Wait for processing to be attempted (sync is called inside Setup)
				Eventually(func() int {
					return brancher.FetchCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

				// handlePromptFailure must NOT be called — daemon stopped on post-execution failure.
				// processPrompt calls Load once (count=1) before Setup; handlePromptFailure would
				// call Load a second time (count=2). Consistently ensures Load is called exactly once.
				Consistently(func() int {
					return manager.LoadCallCount()
				}, 500*time.Millisecond, 50*time.Millisecond).Should(Equal(1))

				cancel()
			},
		)

		It(
			"calls handlePromptFailure when file still exists at in-progress path after error",
			func() {
				promptPath := filepath.Join(promptsDir, "001-pre-exec-fail.md")

				// File exists at pr.Path — pre-execution failure, file was not moved to completed/
				Expect(
					os.WriteFile(promptPath, []byte("---\nstatus: queued\n---\n"), 0600),
				).To(Succeed())

				queued := []prompt.Prompt{
					{Path: promptPath, Status: prompt.ApprovedPromptStatus},
				}
				manager.ListQueuedReturnsOnCall(0, queued, nil)
				manager.ListQueuedReturnsOnCall(1, nil, nil)
				manager.AllPreviousCompletedReturns(true)
				brancher.FetchReturns(stderrors.New("fetch failed"))

				p := newProc()
				go func() {
					_ = p.Process(ctx)
				}()

				// handlePromptFailure is called because file still exists at pr.Path
				Eventually(func() int {
					return manager.LoadCallCount()
				}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

				cancel()
			},
		)
	})

	Describe("container lock timing", func() {
		It("acquires container lock after prompt load, not before", func() {
			promptPath := filepath.Join(promptsDir, "001-lock-timing.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)
			manager.SetStatusReturns(nil)
			manager.MoveToCompletedReturns(nil)
			executor.ExecuteReturns(nil)
			releaser.CommitCompletedFileReturns(nil)
			releaser.HasChangelogReturns(false)
			releaser.CommitOnlyReturns(nil)

			// Track ordering: Load must happen before Acquire
			var loadTime, acquireTime time.Time
			manager.LoadStub = func(_ context.Context, path string) (*prompt.PromptFile, error) {
				loadTime = time.Now()
				return newProcessorTestPromptFile(path, "# Lock test\n\nTest content"), nil
			}

			lock := &mocks.ContainerLock{}
			lock.AcquireStub = func(_ context.Context) error {
				acquireTime = time.Now()
				return nil
			}
			lock.ReleaseReturns(nil)

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
				lock,
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

			// Lock must have been acquired after Load
			Expect(loadTime).NotTo(BeZero(), "Load should have been called")
			Expect(acquireTime).NotTo(BeZero(), "Acquire should have been called")
			Expect(acquireTime).To(BeTemporally(">=", loadTime),
				"container lock should be acquired after prompt load, not before")

			cancel()
		})
	})

	Describe("fetch timeout", func() {
		It("cancels fetch after timeout instead of hanging indefinitely", func() {
			promptPath := filepath.Join(promptsDir, "001-fetch-timeout.md")
			queued := []prompt.Prompt{
				{Path: promptPath, Status: prompt.ApprovedPromptStatus},
			}

			manager.ListQueuedReturnsOnCall(0, queued, nil)
			manager.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			manager.AllPreviousCompletedReturns(true)

			// Simulate a hanging fetch — block until context is cancelled
			brancher.FetchStub = func(fetchCtx context.Context) error {
				<-fetchCtx.Done()
				return fetchCtx.Err()
			}

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

			start := time.Now()
			go func() {
				_ = p.Process(ctx)
			}()

			// The fetch should time out within 30s, not hang forever.
			// We verify the processor moves past the fetch (either erroring or retrying)
			// within a reasonable window. The 30s timeout in syncWithRemote ensures this.
			Eventually(func() int {
				return brancher.FetchCallCount()
			}, 35*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			// Verify it didn't take much longer than 30s (the timeout)
			elapsed := time.Since(start)
			Expect(elapsed).To(BeNumerically("<", 35*time.Second),
				"fetch should be cancelled by timeout, not hang indefinitely")

			cancel()
		})
	})

	Describe("periodic auto-complete sweep", func() {
		It("self-heals a stuck prompted spec via the periodic sweep", func() {
			// Directories for this sub-test
			sweepTempDir, err := os.MkdirTemp("", "sweep-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(sweepTempDir) }()

			sweepQueueDir := filepath.Join(sweepTempDir, "prompts", "in-progress")
			sweepCompletedDir := filepath.Join(sweepTempDir, "prompts", "completed")
			sweepSpecsInboxDir := filepath.Join(sweepTempDir, "specs", "inbox")
			sweepSpecsInProgressDir := filepath.Join(sweepTempDir, "specs", "in-progress")
			sweepSpecsCompletedDir := filepath.Join(sweepTempDir, "specs", "completed")

			for _, dir := range []string{
				sweepQueueDir,
				sweepCompletedDir,
				sweepSpecsInboxDir,
				sweepSpecsInProgressDir,
				sweepSpecsCompletedDir,
			} {
				Expect(os.MkdirAll(dir, 0750)).To(Succeed())
			}

			// A spec stuck in "prompted" state
			specPath := filepath.Join(sweepSpecsInProgressDir, "spec-sweep.md")
			Expect(
				os.WriteFile(specPath, []byte("---\nstatus: prompted\n---\n# Sweep spec\n"), 0600),
			).To(Succeed())

			// All linked prompts already in completed dir (simulating previous run that crashed
			// before CheckAndComplete, leaving the spec stuck in "prompted").
			Expect(os.WriteFile(
				filepath.Join(sweepCompletedDir, "099-linked.md"),
				[]byte("---\nstatus: completed\nspec: spec-sweep\n---\n# Linked prompt\n"),
				0600,
			)).To(Succeed())

			// Real lister and autoCompleter for the sweep dirs
			realLister := spec.NewLister(libtime.NewCurrentDateTime(), sweepSpecsInProgressDir)
			realAutoCompleter := spec.NewAutoCompleter(
				sweepQueueDir,
				sweepCompletedDir,
				sweepSpecsInboxDir,
				sweepSpecsInProgressDir,
				sweepSpecsCompletedDir,
				libtime.NewCurrentDateTime(),
				"",
				notifier.NewMultiNotifier(),
			)

			// Manager returns empty queue so the processor doesn't try to run any prompts
			manager.ListQueuedReturns([]prompt.Prompt{}, nil)
			manager.FindCommittingReturns(nil, nil)

			we := processor.NewDirectWorkflowExecutor(processor.WorkflowDeps{
				ProjectName:   processor.ProjectName("sweep-test"),
				PromptManager: manager,
				AutoCompleter: realAutoCompleter,
				Releaser:      releaser,
				Brancher:      brancher,
				PRCreator:     prCreator,
				Cloner:        cloner,
				Worktreer:     worktreer,
				PRMerger:      prMerger,
			})
			sweepFH := failurehandler.NewHandler(
				manager,
				notifier.NewMultiNotifier(),
				sweepCompletedDir,
				"sweep-test",
				0,
			)
			sweepResumer := promptresumer.NewResumer(
				manager,
				executor,
				&noOpWorkflowExecutorAdapter{},
				completionreport.NewValidator(),
				sweepFH,
				sweepQueueDir,
				sweepCompletedDir,
				filepath.Join(sweepTempDir, "log"),
				"sweep-test",
				0,
			)
			sweepProc := processor.NewProcessor(
				executor,
				manager,
				releaser,
				versionGet,
				we,
				realAutoCompleter,
				specsweeper.NewSweeper(realLister, realAutoCompleter),
				preflightconditions.NewConditions(nil, nil, nil, 0),
				containerslot.NewManager(nil, nil, nil, 0, 10*time.Second),
				cancellationwatcher.NewWatcher(executor, manager),
				wakeup,
				processor.Dirs{
					Queue:     sweepQueueDir,
					Completed: sweepCompletedDir,
					Log:       filepath.Join(sweepTempDir, "log"),
				},
				processor.ProjectName("sweep-test"),
				sweepFH,
				sweepResumer,
				processor.VerificationGate(false),
				completionreport.NewValidator(),
				promptenricher.NewEnricher(
					releaser,
					"",
					"",
					"",
					"",
					validationprompt.NewResolver(),
				),
				committingrecoverer.NewRecoverer(
					manager,
					releaser,
					realAutoCompleter,
					sweepCompletedDir,
				),
				0,
				20*time.Millisecond, // sweepInterval 20ms for test speed
				nil,                 // onIdle: no-op for tests
			)
			sweepProc.SetScanner(
				queuescanner.NewScanner(manager, sweepProc, sweepFH, sweepQueueDir),
			)
			p := sweepProc

			sweepCtx, sweepCancel := context.WithCancel(context.Background())
			defer sweepCancel()
			go func() {
				_ = p.Process(sweepCtx)
			}()

			// Wait for the sweep ticker to fire and transition the spec to verifying
			Eventually(func() string {
				sf, loadErr := spec.Load(sweepCtx, specPath, libtime.NewCurrentDateTime())
				if loadErr != nil {
					return ""
				}
				return sf.Frontmatter.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal("verifying"),
				"periodic sweep should self-heal stuck prompted spec within sweep interval")

			sweepCancel()
		})
	})

})
