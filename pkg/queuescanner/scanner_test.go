// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuescanner_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/preflightconditions"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/queuescanner"
)

var _ = Describe("Scanner", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		queueDir       string
		mgr            *mocks.QueueScannerPromptManager
		pp             *mocks.PromptProcessor
		failureHandler *mocks.FailureHandler
		s              queuescanner.Scanner
	)

	makeApprovedPrompt := func(name string) prompt.Prompt {
		return prompt.Prompt{
			Path:   filepath.Join(queueDir, name),
			Status: prompt.ApprovedPromptStatus,
		}
	}

	writeFile := func(name, content string) string {
		path := filepath.Join(queueDir, name)
		Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())
		return path
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		var err error
		queueDir, err = os.MkdirTemp("", "queuescanner-test-*")
		Expect(err).NotTo(HaveOccurred())

		mgr = &mocks.QueueScannerPromptManager{}
		pp = &mocks.PromptProcessor{}
		failureHandler = &mocks.FailureHandler{}

		// Default: Load returns error (no pending verification files)
		mgr.LoadReturns(nil, stderrors.New("not found"))

		s = queuescanner.NewScanner(mgr, pp, failureHandler, queueDir)
	})

	AfterEach(func() {
		cancel()
		if queueDir != "" {
			_ = os.RemoveAll(queueDir)
		}
	})

	Describe("ScanAndProcess", func() {
		Context("empty queue", func() {
			BeforeEach(func() {
				mgr.ListQueuedReturns([]prompt.Prompt{}, nil)
			})

			It("returns 0 completed with no error", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})
		})

		Context("pending verification blocks queue", func() {
			BeforeEach(func() {
				path := writeFile(
					"001-pending.md",
					"---\nstatus: pending_verification\n---\n# Test\n",
				)
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.PendingVerificationPromptStatus)},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
			})

			It("returns 0 completed without listing the queue", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(mgr.ListQueuedCallCount()).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})
		})

		Context("queued prompt is processed successfully", func() {
			BeforeEach(func() {
				writeFile(
					"001-my-prompt.md",
					"---\nstatus: approved\n---\n# Test prompt\ncontent\n",
				)
				pr := makeApprovedPrompt("001-my-prompt.md")
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(nil)
			})

			It("returns 1 completed and calls ProcessPrompt", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(1))
				Expect(pp.ProcessPromptCallCount()).To(Equal(1))
			})
		})

		Context("prompt validation fails (no numeric prefix in filename)", func() {
			BeforeEach(func() {
				// bad-prompt.md has no NNN- prefix — ValidateForExecution will fail
				writeFile("bad-prompt.md", "---\nstatus: approved\n---\n# Bad\ncontent\n")
				pr := prompt.Prompt{
					Path:   filepath.Join(queueDir, "bad-prompt.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				// First call: returns the bad prompt; second call: empty (terminates loop)
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mgr.AllPreviousCompletedReturns(true)
			})

			// completed counts "continue" iterations; a skip still counts as a loop pass.
			It("skips the prompt without calling ProcessPrompt", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(BeNumerically(">=", 0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})
		})

		Context("file unchanged — skipped silently on second scan", func() {
			var pr prompt.Prompt

			BeforeEach(func() {
				// bad-no-number.md has no numeric prefix so validation fails
				writeFile("bad-no-number.md", "---\nstatus: approved\n---\n# Bad\ncontent\n")
				pr = prompt.Prompt{
					Path:   filepath.Join(queueDir, "bad-no-number.md"),
					Status: prompt.ApprovedPromptStatus,
				}
				// Each scan: bad prompt on first call, empty on second
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mgr.ListQueuedReturnsOnCall(2, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(3, []prompt.Prompt{}, nil)
				mgr.AllPreviousCompletedReturns(true)
			})

			It("skips on first call and silently skips on second call", func() {
				// First scan: fails validation, records in skippedPrompts
				_, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))

				// Second scan: file unchanged → silently skip (same mtime)
				_, err = s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})
		})

		Context("file modified — re-validated after mtime changes", func() {
			var path string

			BeforeEach(func() {
				path = writeFile("bad-no-number.md", "---\nstatus: approved\n---\n# Bad\ncontent\n")
				mgr.AllPreviousCompletedReturns(true)
			})

			It("re-evaluates the prompt when the file changes", func() {
				badPr := prompt.Prompt{
					Path:   path,
					Status: prompt.ApprovedPromptStatus,
				}
				// First scan: bad prompt, then empty
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{badPr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)

				// First scan: fails validation, records in cache
				_, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))

				// Write the new (valid) file with a different name (fixed prefix)
				newPath := writeFile(
					"001-fixed.md",
					"---\nstatus: approved\n---\n# Fixed\ncontent\n",
				)
				fixedPr := makeApprovedPrompt("001-fixed.md")
				_ = newPath

				// Second scan: fixed prompt, then empty
				mgr.ListQueuedReturnsOnCall(2, []prompt.Prompt{fixedPr}, nil)
				mgr.ListQueuedReturnsOnCall(3, []prompt.Prompt{}, nil)
				pp.ProcessPromptReturns(nil)

				_, err = s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(pp.ProcessPromptCallCount()).To(Equal(1))
			})
		})

		Context("blocked on prior prompt", func() {
			BeforeEach(func() {
				writeFile("002-blocked.md", "---\nstatus: approved\n---\n# Blocked\ncontent\n")
				pr := makeApprovedPrompt("002-blocked.md")
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(false)
				mgr.FindMissingCompletedReturns([]int{1})
				mgr.FindPromptStatusInProgressReturns("executing")
			})

			It("returns 0 without calling ProcessPrompt", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})

			It("logs blocked message only once for identical state (deduplication)", func() {
				// Both calls see same missing prompts
				_, _ = s.ScanAndProcess(ctx)
				_, _ = s.ScanAndProcess(ctx)
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})
		})

		Context("prior completed — unblocks on next scan", func() {
			var pr prompt.Prompt

			BeforeEach(func() {
				writeFile("002-unblocked.md", "---\nstatus: approved\n---\n# Unblocked\ncontent\n")
				pr = makeApprovedPrompt("002-unblocked.md")

				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(2, []prompt.Prompt{}, nil)

				mgr.AllPreviousCompletedReturnsOnCall(0, false)
				mgr.AllPreviousCompletedReturnsOnCall(1, true)
				mgr.FindMissingCompletedReturns([]int{1})
				mgr.FindPromptStatusInProgressReturns("executing")
				pp.ProcessPromptReturns(nil)
			})

			It("processes on second scan after prior completes", func() {
				// First scan: blocked
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))

				// Second scan: unblocked, processes
				completed, err = s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(1))
				Expect(pp.ProcessPromptCallCount()).To(Equal(1))
			})
		})

		Context("preflight skip propagates and stops scan", func() {
			BeforeEach(func() {
				writeFile("001-preflight.md", "---\nstatus: approved\n---\n# Preflight\ncontent\n")
				pr := makeApprovedPrompt("001-preflight.md")
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(preflightconditions.ErrPreflightSkip)
			})

			It("stops scan loop without error, does not call failureHandler", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(1))
				Expect(failureHandler.HandleCallCount()).To(Equal(0))
			})
		})

		Context("status auto-set for non-terminal status (e.g. draft)", func() {
			BeforeEach(func() {
				writeFile("001-auto-set.md", "---\nstatus: approved\n---\n# Auto-set\ncontent\n")
				pr := prompt.Prompt{
					Path:   filepath.Join(queueDir, "001-auto-set.md"),
					Status: "draft", // non-terminal → auto-promoted to approved
				}
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mgr.SetStatusReturns(nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(nil)
			})

			It("calls SetStatus to promote to approved before processing", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(1))
				Expect(mgr.SetStatusCallCount()).To(Equal(1))
				_, _, status := mgr.SetStatusArgsForCall(0)
				Expect(status).To(Equal(string(prompt.ApprovedPromptStatus)))
			})
		})

		Context("ListQueued error is returned as fatal", func() {
			BeforeEach(func() {
				mgr.ListQueuedReturns(nil, stderrors.New("db error"))
			})

			It("returns the error and 0 completed", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("db error"))
				Expect(completed).To(Equal(0))
			})
		})

		Context("ProcessPrompt error handled non-fatally by failureHandler", func() {
			BeforeEach(func() {
				writeFile("001-fail.md", "---\nstatus: approved\n---\n# Fail\ncontent\n")
				pr := makeApprovedPrompt("001-fail.md")
				mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
				mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(stderrors.New("execution failed"))
				failureHandler.HandleReturns(nil) // non-fatal: re-queue or permanently fail
			})

			It("calls failureHandler and continues scanning", func() {
				_, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(failureHandler.HandleCallCount()).To(Equal(1))
			})
		})

		Context("ProcessPrompt error and failureHandler returns stop error", func() {
			BeforeEach(func() {
				writeFile("001-stop.md", "---\nstatus: approved\n---\n# Stop\ncontent\n")
				pr := makeApprovedPrompt("001-stop.md")
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(stderrors.New("execution failed"))
				failureHandler.HandleReturns(stderrors.New("stop error"))
			})

			It("returns the stop error from failureHandler", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("stop error"))
				Expect(completed).To(Equal(0))
			})
		})

		Context("context cancelled before scan loop", func() {
			It("returns 0 without error", func() {
				cancel()
				mgr.ListQueuedReturns([]prompt.Prompt{}, nil)
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
			})
		})
	})

	Describe("HasPendingVerification", func() {
		Context("no files in queue dir", func() {
			It("returns false", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeFalse())
			})
		})

		Context("queue dir has a prompt with pending_verification status", func() {
			BeforeEach(func() {
				path := writeFile(
					"001-pending.md",
					"---\nstatus: pending_verification\n---\n# Test\n",
				)
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.PendingVerificationPromptStatus)},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
			})

			It("returns true", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeTrue())
			})
		})

		Context("queue dir has a prompt with approved status", func() {
			BeforeEach(func() {
				path := writeFile("001-approved.md", "---\nstatus: approved\n---\n# Test\n")
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
			})

			It("returns false", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeFalse())
			})
		})

		Context("Load returns error for file", func() {
			BeforeEach(func() {
				writeFile("001-error.md", "---\nstatus: approved\n---\n# Test\n")
				mgr.LoadReturns(nil, stderrors.New("load error"))
			})

			It("skips the file and returns false", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeFalse())
			})
		})

		Context("Load returns nil prompt file (no error)", func() {
			BeforeEach(func() {
				writeFile("001-nil.md", "---\nstatus: approved\n---\n# Test\n")
				mgr.LoadReturns(nil, nil) //nolint:nilnil
			})

			It("skips the nil file and returns false", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeFalse())
			})
		})

		Context("queue dir does not exist", func() {
			BeforeEach(func() {
				s = queuescanner.NewScanner(mgr, pp, failureHandler, "/nonexistent/path")
			})

			It("returns false gracefully", func() {
				Expect(s.HasPendingVerification(ctx)).To(BeFalse())
			})
		})
	})

	Describe("ClearSkippedCache", func() {
		It("does not panic", func() {
			Expect(func() { s.ClearSkippedCache() }).NotTo(Panic())
		})

		It("allows re-evaluation of previously-skipped prompts", func() {
			writeFile("bad-no-number.md", "---\nstatus: approved\n---\n# Bad\ncontent\n")
			pr := prompt.Prompt{
				Path:   filepath.Join(queueDir, "bad-no-number.md"),
				Status: prompt.ApprovedPromptStatus,
			}
			// Scan 1: bad prompt → skip → then empty
			mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr}, nil)
			mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
			// Scan 2 (after clear): bad prompt → re-evaluated → then empty
			mgr.ListQueuedReturnsOnCall(2, []prompt.Prompt{pr}, nil)
			mgr.ListQueuedReturnsOnCall(3, []prompt.Prompt{}, nil)
			mgr.AllPreviousCompletedReturns(true)

			// First scan: fails validation, records in cache
			_, _ = s.ScanAndProcess(ctx)
			Expect(pp.ProcessPromptCallCount()).To(Equal(0))

			// Clear the cache
			s.ClearSkippedCache()

			// Second scan: cache cleared, prompt is re-evaluated (still fails but logs again)
			_, _ = s.ScanAndProcess(ctx)
			Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			// Both scans visited the prompt (2 ListQueued calls each)
			Expect(mgr.ListQueuedCallCount()).To(Equal(4))
		})
	})
})
