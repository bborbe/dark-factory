// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

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
				false, // autoMerge disabled
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
				true, // autoRelease enabled
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
					true, // autoRelease enabled
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
				true,  // autoMerge enabled
				false, // autoRelease disabled
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
					false, // autoMerge disabled
					false,
					true, // autoReview enabled
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
				false, // autoMerge disabled
				false,
				false, // autoReview disabled
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

})
