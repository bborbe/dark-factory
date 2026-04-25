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
	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/processor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
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
					true, // verificationGate enabled
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
					true, // verificationGate enabled
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
					true, // gate enabled but no pending file
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
				realLister,
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

			// Wait for CheckAndComplete to be called with the spec name
			Eventually(func() int {
				return autoCompleter.CheckAndCompleteCallCount()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(1))

			_, specID := autoCompleter.CheckAndCompleteArgsForCall(0)
			Expect(specID).To(Equal("001-my-spec"))

			cancel()
		})
	})

	Describe("Process log output", func() {
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

})
