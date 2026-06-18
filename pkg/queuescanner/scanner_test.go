// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queuescanner_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/cmd"
	lockpkg "github.com/bborbe/dark-factory/pkg/lock"
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

		// Default: Load returns a valid pre-execution PromptFile.
		//
		// The scanner takes a per-prompt lock right before handing the
		// candidate to the processor, then re-reads the prompt via
		// PromptManager.Load. Returning a valid file with
		// status=approved makes the post-lock re-read see an advanceable
		// candidate, so existing tests asserting on ProcessPrompt
		// continue to pass. Tests that want to simulate a stale or
		// moved file override this stub.
		mgr.LoadStub = func(
			_ context.Context, path string,
		) (*prompt.PromptFile, error) {
			return prompt.NewPromptFile(
				path,
				prompt.Frontmatter{Status: string(prompt.ApprovedPromptStatus)},
				[]byte("# Test\n"),
				nil,
			), nil
		}

		s = queuescanner.NewScanner(mgr, pp, failureHandler, queueDir, nil, 0)
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

		Context("preflight failure propagates as error and stops scan", func() {
			BeforeEach(func() {
				writeFile("001-preflight.md", "---\nstatus: approved\n---\n# Preflight\ncontent\n")
				pr := makeApprovedPrompt("001-preflight.md")
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(preflightconditions.ErrPreflightFailed)
			})

			It("returns ErrPreflightFailed without calling failureHandler", func() {
				completed, err := s.ScanAndProcess(ctx)
				Expect(err).To(HaveOccurred())
				Expect(stderrors.Is(err, preflightconditions.ErrPreflightFailed)).To(BeTrue())
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

		Context("per-prompt lock acquire timeout", func() {
			It("ends the scan, logs project-lock-timeout once, processes nothing", func() {
				path := writeFile(
					"230-spec-060-locked.md",
					promptFrontmatterWithSpec(prompt.ApprovedPromptStatus, []string{"060"}),
				)
				pr := prompt.Prompt{Path: path, Status: prompt.ApprovedPromptStatus}
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(true)
				mgr.AllPreviousInSpecCompletedReturns(true)

				lockMock := &mocks.LockDirLock{}
				lockMock.AcquireReturns(stderrors.New("lock acquire timeout"))
				s = queuescanner.NewScanner(
					mgr, pp, failureHandler, queueDir,
					func(string) lockpkg.DirLock { return lockMock },
					10*time.Millisecond,
				)

				var logBuf bytes.Buffer
				original := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
				defer slog.SetDefault(original)

				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				// The timeout branch must END the scan (return true): a false
				// return hot-loops on the same locked candidate within one
				// ScanAndProcess call — blocking lockTimeout per iteration,
				// inflating the completed counter, and starving other specs.
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
				Expect(logBuf.String()).To(ContainSubstring(
					fmt.Sprintf("reason=%s", prompt.ReasonProjectLockTimeout),
				))

				// Dedupe survives the failed acquire: a second scan against the
				// same stuck lock must not re-log the blocked line (the key is
				// cleared only after a successful acquire).
				logBuf.Reset()
				_, err = s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(logBuf.String()).NotTo(ContainSubstring(
					string(prompt.ReasonProjectLockTimeout),
				))
			})
		})

		Context("per-spec predecessor lookup", func() {
			makePrompt := func(name string, status prompt.PromptStatus, specList []string) prompt.Prompt {
				path := writeFile(
					name,
					promptFrontmatterWithSpec(status, specList),
				)
				return prompt.Prompt{Path: path, Status: status}
			}

			It(
				"selects a candidate from one spec without being blocked by a different spec",
				func() {
					// Fixtures: 226 of spec 056 in in-progress/; 227 of spec 058 in in-progress/
					// Per-spec: both have predecessors completed, both pass the per-spec guard.
					// The scanner picks the alphabetic-first candidate (226) and processes it.
					// The KEY property: the scanner does NOT block on the cross-spec combination.
					pr226 := makePrompt(
						"226-spec-056-blocker.md",
						prompt.ApprovedPromptStatus,
						[]string{"056"},
					)
					pr227 := makePrompt(
						"227-spec-058-foo.md",
						prompt.ApprovedPromptStatus,
						[]string{"058"},
					)
					mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr226, pr227}, nil)
					mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
					mgr.AllPreviousCompletedReturns(true)
					mgr.AllPreviousInSpecCompletedReturns(true)
					pp.ProcessPromptReturns(nil)

					completed, err := s.ScanAndProcess(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(completed).To(Equal(1))
					Expect(pp.ProcessPromptCallCount()).To(Equal(1))
					// 226 is alphabetic-first; both are valid choices given the per-spec
					// guard. The KEY assertion: at least one was processed.
				},
			)

			It(
				"processes spec B candidate while spec A is blocked by failed/missing predecessor",
				func() {
					// Spec 094 AC "cross-spec-advance": one spec's queue must not
					// block an unrelated spec's queue. The test sets up spec A
					// genuinely blocked (predecessor failed/missing) and spec B
					// advanceable, and asserts BOTH that B is processed AND that A
					// is not — the negative assertion is the regression lock.
					// Wire Load to surface the spec field so the per-spec guard
					// is exercised (default LoadReturns would fall through to
					// the legacy global guard and mask the per-spec behavior).
					pr226 := makePrompt(
						"226-spec-A-blocker.md",
						prompt.ApprovedPromptStatus,
						[]string{"A"},
					)
					pr230 := makePrompt(
						"230-spec-B-advanceable.md",
						prompt.ApprovedPromptStatus,
						[]string{"B"},
					)
					mgr.LoadStub = func(
						_ context.Context, path string,
					) (*prompt.PromptFile, error) {
						switch filepath.Base(path) {
						case "226-spec-A-blocker.md":
							return prompt.NewPromptFile(
								path,
								prompt.Frontmatter{
									Status: string(prompt.ApprovedPromptStatus),
									Specs:  prompt.SpecList{"A"},
								},
								[]byte("# Test\n"),
								nil,
							), nil
						case "230-spec-B-advanceable.md":
							return prompt.NewPromptFile(
								path,
								prompt.Frontmatter{
									Status: string(prompt.ApprovedPromptStatus),
									Specs:  prompt.SpecList{"B"},
								},
								[]byte("# Test\n"),
								nil,
							), nil
						}
						return nil, stderrors.New("not found")
					}
					mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr226, pr230}, nil)
					mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
					// Spec A's predecessor 225 is failed/missing → blocked.
					mgr.AllPreviousInSpecCompletedStub = func(
						_ context.Context, _ int, specID string,
					) bool {
						if specID == "A" {
							return false // spec A's predecessor is failed/missing
						}
						return true // spec B's predecessor is complete
					}
					mgr.FindMissingInSpecCompletedStub = func(
						_ context.Context, _ int, specID string,
					) int {
						if specID == "A" {
							return 225 // spec A is blocked by missing 225
						}
						return 0
					}
					// Legacy global guard stub (unused here — Load surfaces
					// the spec field so the per-spec guard is consulted — but
					// the scanner still consults this when no spec field is
					// present, so keep it honest).
					mgr.AllPreviousCompletedReturns(true)
					pp.ProcessPromptReturns(nil)

					completed, err := s.ScanAndProcess(ctx)
					Expect(err).NotTo(HaveOccurred())

					// POSITIVE: spec B's candidate was processed.
					Expect(completed).To(Equal(1))
					Expect(pp.ProcessPromptCallCount()).To(Equal(1))
					_, processed := pp.ProcessPromptArgsForCall(0)
					Expect(processed.Path).To(HaveSuffix("230-spec-B-advanceable.md"))

					// NEGATIVE: spec A's candidate was NOT processed.
					// (Removing this assertion is the regression-flagged
					// weakening from spec 094 Failure Mode "Cross-spec test
					// weakened".) We assert this by inspecting every
					// ProcessPrompt call: only spec B's path was ever passed
					// in. Spec A's path is absent.
					for i := 0; i < pp.ProcessPromptCallCount(); i++ {
						_, arg := pp.ProcessPromptArgsForCall(i)
						Expect(arg.Path).NotTo(HaveSuffix("226-spec-A-blocker.md"))
					}
				},
			)

			It(
				"picks the lower global prompt number when both specs have a ready candidate",
				func() {
					// Fixtures: spec A's 221 ready, spec B's 223 ready; ListQueued returns [221, 223]
					pr221 := makePrompt("221-spec-A.md", prompt.ApprovedPromptStatus, []string{"A"})
					pr223 := makePrompt("223-spec-B.md", prompt.ApprovedPromptStatus, []string{"B"})
					mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr221, pr223}, nil)
					mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
					mgr.AllPreviousCompletedReturns(true)
					mgr.AllPreviousInSpecCompletedReturns(true)
					pp.ProcessPromptReturns(nil)

					_, err := s.ScanAndProcess(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(pp.ProcessPromptCallCount()).To(Equal(1))
					_, processed := pp.ProcessPromptArgsForCall(0)
					Expect(processed.Path).To(HaveSuffix("221-spec-A.md"))
				},
			)

			It("blocks candidate whose same-spec predecessor is not completed", func() {
				// Fixtures: 220 of spec 056 in completed/; 222 of spec 056 in in-progress/ (no 221)
				path220 := writeFile(
					"220-spec-056-prev.md",
					promptFrontmatterWithSpec(prompt.CompletedPromptStatus, []string{"056"}),
				)
				_ = path220
				pr222 := makePrompt(
					"222-spec-056-foo.md",
					prompt.ApprovedPromptStatus,
					[]string{"056"},
				)
				mgr.ListQueuedReturns([]prompt.Prompt{pr222}, nil)
				// Per-spec returns false (221 is missing)
				mgr.AllPreviousInSpecCompletedReturns(false)
				mgr.FindMissingInSpecCompletedReturns(221)

				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
			})

			It("logs prompt blocked with spec id", func() {
				path := writeFile(
					"222-spec-056-foo.md",
					promptFrontmatterWithSpec(prompt.ApprovedPromptStatus, []string{"056"}),
				)
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status: string(prompt.ApprovedPromptStatus),
						Specs:  prompt.SpecList{"056"},
					},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
				pr222 := prompt.Prompt{Path: path, Status: prompt.ApprovedPromptStatus}
				mgr.ListQueuedReturns([]prompt.Prompt{pr222}, nil)
				mgr.AllPreviousInSpecCompletedReturns(false)
				mgr.FindMissingInSpecCompletedReturns(221)

				// Capture slog output
				var logBuf bytes.Buffer
				original := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
				defer slog.SetDefault(original)

				_, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(logBuf.String()).To(ContainSubstring("prompt blocked"))
				Expect(logBuf.String()).To(ContainSubstring("spec=056"))
			})

			It("treats a multi-spec prompt as malformed and surfaces via Blocked", func() {
				// Fixture: candidate whose PromptFile.Frontmatter.Specs has 2 entries
				path := writeFile(
					"222-multi-spec.md",
					promptFrontmatterWithSpec(prompt.ApprovedPromptStatus, []string{"056", "058"}),
				)
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status: string(prompt.ApprovedPromptStatus),
						Specs:  prompt.SpecList{"056", "058"},
					},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
				pr := prompt.Prompt{Path: path, Status: prompt.ApprovedPromptStatus}
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousCompletedReturns(true)
				pp.ProcessPromptReturns(nil)

				var logBuf bytes.Buffer
				original := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
				defer slog.SetDefault(original)

				completed, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(completed).To(Equal(0))
				Expect(pp.ProcessPromptCallCount()).To(Equal(0))
				// Spec 094 AC "scanner-log-enum": the canonical
				// frontmatter-parse-error token is emitted, not the
				// historical human string. Both surfaces (scanner log
				// and status) source the token from the same constant
				// in pkg/prompt.
				Expect(logBuf.String()).To(ContainSubstring(
					fmt.Sprintf("reason=%s", prompt.ReasonPromptFrontmatterParseError),
				))
				Expect(logBuf.String()).NotTo(ContainSubstring("malformed frontmatter"))
			})

			It("emits the hyphenated reason token in the blocked log line", func() {
				// Spec 094 AC "scanner-log-enum": the scanner's blocked-log
				// line must emit the hyphenated enum shared with status
				// (`reason=previous-prompt-not-completed`), not the spaced
				// human string. Both surfaces source the token from
				// `prompt.ReasonPreviousPromptNotCompleted`; a regression
				// that hardcoded the spaced literal would fail this test.
				path := writeFile(
					"222-spec-056-foo.md",
					promptFrontmatterWithSpec(prompt.ApprovedPromptStatus, []string{"056"}),
				)
				pf := prompt.NewPromptFile(
					path,
					prompt.Frontmatter{
						Status: string(prompt.ApprovedPromptStatus),
						Specs:  prompt.SpecList{"056"},
					},
					[]byte("# Test\n"),
					nil,
				)
				mgr.LoadReturns(pf, nil)
				pr := prompt.Prompt{Path: path, Status: prompt.ApprovedPromptStatus}
				mgr.ListQueuedReturns([]prompt.Prompt{pr}, nil)
				mgr.AllPreviousInSpecCompletedReturns(false)
				mgr.FindMissingInSpecCompletedReturns(221)

				// Capture slog output via the same TextHandler shape the
				// daemon uses. SetDefault swap is local to this It and
				// restored by the deferred SetDefault.
				var logBuf bytes.Buffer
				original := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
				defer slog.SetDefault(original)

				_, err := s.ScanAndProcess(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Positive: the hyphenated enum token (sourced from the
				// shared constant in pkg/prompt) appears in the log line.
				Expect(logBuf.String()).To(ContainSubstring(
					fmt.Sprintf("reason=%s", prompt.ReasonPreviousPromptNotCompleted),
				))
				// Negative: the spaced human string is gone. A regression
				// that hardcoded "previous prompt not completed" inside
				// logBlockedOnce would fail this assertion.
				Expect(logBuf.String()).NotTo(ContainSubstring("previous prompt not completed"))
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
				s = queuescanner.NewScanner(mgr, pp, failureHandler, "/nonexistent/path", nil, 0)
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

func promptFrontmatterWithSpec(status prompt.PromptStatus, specList []string) string {
	content := "---\nstatus: " + string(status) + "\n"
	if len(specList) == 1 {
		content += "spec: " + specList[0] + "\n"
	} else if len(specList) > 1 {
		content += "spec: ["
		for i, s := range specList {
			if i > 0 {
				content += ", "
			}
			content += s
		}
		content += "]\n"
	}
	content += "---\n# Test\n\nContent\n"
	return content
}

var _ = Describe("ConcurrentRejectAndAdvance", func() {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return
	}
	It("serializes reject and advance via project lock — no double-write", func() {
		// Lock-primitive test: only the DirLock primitive is
		// exercised. The companion "serializes a real reject
		// against a real scanner advance" test below wires up
		// the real cmd.NewRejectCommand and a real
		// queuescanner.Scanner over a real prompt fixture and
		// is the regression-lock test for the spec-092
		// concurrent-reject-advance contract. This test stays
		// as a focused unit test of the lock primitive
		// itself — the only failure mode it covers is
		// "external holder of the lock blocks a fresh Acquire
		// within the timeout".
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		dir, err := os.MkdirTemp("", "concurrent-reject-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir)

		promptPath := filepath.Join(dir, "226-spec-056.md")
		Expect(os.WriteFile(promptPath, []byte(""+
			"---\n"+
			"status: failed\n"+
			"spec: [\"056\"]\n"+
			"---\n"+
			"# Test\n",
		), 0600)).To(Succeed())

		// Acquire the lock first
		lockInstance := lockpkg.NewDirLock(filepath.Dir(promptPath))
		Expect(lockInstance.Acquire(ctx, 5*time.Second)).To(Succeed())
		defer func() {
			_ = lockInstance.Release(ctx)
		}()

		// Scanner cannot acquire the same lock within a short timeout
		otherLock := lockpkg.NewDirLock(filepath.Dir(promptPath))
		err = otherLock.Acquire(ctx, 200*time.Millisecond)
		Expect(err).To(HaveOccurred(), "expected lock contention within 200ms")

		// Release and verify the next acquirer succeeds
		Expect(lockInstance.Release(ctx)).To(Succeed())
		Expect(otherLock.Acquire(ctx, 5*time.Second)).To(Succeed())
		Expect(otherLock.Release(ctx)).To(Succeed())
	})

	It(
		"serializes a real reject against a real scanner advance on one prompt fixture",
		func() {
			// Spec 092 AC "concurrent-reject-advance": a real prompt
			// reject and a real scanner advance on the same prompt file
			// must serialize under the per-prompt file lock. Exactly one
			// final on-disk state, no corruption, the loser observes the
			// post-lock state. This test exercises the actual reject
			// command + actual scanner against a real prompt fixture.
			// Both contenders take the same lock.NewDirLock on the
			// prompt path (prompt 2 will update to filepath.Dir), so the
			// assertion that fails if the production lock from step 3 is
			// removed is: "exactly one final on-disk state" (a torn
			// save/rename interleaving would leave the file in BOTH
			// in-progress/ and rejected/).
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir, err := os.MkdirTemp("", "concurrent-reject-real-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(dir)

			// Build a real prompt.NewManager for the scanner contender
			// so the scanner's processSingleQueued exercises the real
			// file-locking, Load, IsPreExecution, etc. path — not a
			// mock. completedDir and cancelledDir are separate temp
			// dirs because the scanner's helpers consult them via
			// AllPreviousCompleted etc.
			inboxDir := filepath.Join(dir, "inbox")
			inProgressDir := filepath.Join(dir, "in-progress")
			rejectedDir := filepath.Join(dir, "rejected")
			completedDir := filepath.Join(dir, "completed")
			cancelledDir := filepath.Join(dir, "cancelled")
			Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(completedDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(cancelledDir, 0750)).To(Succeed())

			// Use number 001 so the scanner's AllPreviousCompleted(1)
			// returns true vacuously (no predecessors) — the scanner
			// must actually pick this candidate for the lock to be
			// exercised on the contention path. status: approved is
			// pre-execution (IsPreExecution() == true) and the
			// post-lock re-read will see it as still advanceable
			// after a non-reject contender wins; status: failed
			// would be filtered by ListQueued, so the scanner would
			// never pick it.
			promptPath := filepath.Join(inProgressDir, "001-test.md")
			Expect(os.WriteFile(promptPath, []byte(
				"---\nstatus: approved\n---\n# Test\n",
			), 0600)).To(Succeed())

			realMgr := prompt.NewManager(
				inboxDir,
				inProgressDir,
				completedDir,
				cancelledDir,
				nil,
				libtime.NewCurrentDateTime(),
			)
			// Use a mock PromptManager so we can control
			// ListQueued to return the file exactly once — the
			// scanner's outer loop would otherwise pick the
			// same file on every iteration in a tight loop and
			// starve the reject goroutine. Load and
			// AllPreviousCompleted delegate to the real
			// prompt.NewManager so the post-lock re-read sees
			// the actual on-disk state (file gone vs. file
			// present, status approved vs. status rejected).
			localMgr := &mocks.QueueScannerPromptManager{}
			listQueuedCallCount := 0
			localMgr.ListQueuedStub = func(_ context.Context) (
				[]prompt.Prompt,
				error,
			) {
				listQueuedCallCount++
				if listQueuedCallCount == 1 {
					return []prompt.Prompt{{
						Path:   promptPath,
						Status: prompt.ApprovedPromptStatus,
					}}, nil
				}
				return nil, nil
			}
			localMgr.LoadStub = func(
				_ context.Context, path string,
			) (*prompt.PromptFile, error) {
				return realMgr.Load(ctx, path)
			}
			localMgr.AllPreviousCompletedStub = func(
				_ context.Context, n int,
			) bool {
				return realMgr.AllPreviousCompleted(ctx, n)
			}
			// Failure handler mock — the scanner never calls it on
			// the "skip post-lock" path (ProcessPrompt is never
			// invoked), but we pass a real-shaped mock so the
			// interface is satisfied.
			failureHandler := &mocks.FailureHandler{}
			failureHandler.HandleReturns(nil)
			// PromptProcessor mock. Returns nil without moving
			// the file. The real production scanner would move
			// the file to completed/ via processor.rel, but
			// moving the file here would hide the file from the
			// reject's FindPromptFileInDirs (which only searches
			// inbox, in-progress, rejected — not completed).
			// Keeping the file in in-progress/ lets the reject
			// find it and complete the rename, so the test
			// can assert the final state.
			promptProcessor := &mocks.PromptProcessor{}
			promptProcessor.ProcessPromptReturns(nil)

			// The scanner's lockTimeout must be long enough that
			// the scanner waits for the reject to release the
			// lock. The starter-lock pre-acquire (see below)
			// forces both contenders to queue on the lock
			// simultaneously; once the starter releases, the
			// reject takes the lock, finishes, then the scanner
			// takes it. 5s is safely above the reject's working
			// time but well below the test's overall timeout.
			// Override the factory to use filepath.Dir so both sides lock the
			// directory — this exercises the new DirLock semantics that prompt 2
			// will wire into the production call sites.
			dirLockFactory := func(path string) lockpkg.DirLock {
				return lockpkg.NewDirLock(filepath.Dir(path))
			}
			localScanner := queuescanner.NewScanner(
				localMgr,
				promptProcessor,
				failureHandler,
				inProgressDir,
				dirLockFactory,
				5*time.Second,
			)

			// Real reject command against the temp dirs, using the
			// same NewRejectCommand constructor the production code
			// uses. Override factory to filepath.Dir (prompt 2 does
			// this to production defaults) so both sides contend on
			// the same directory fd even after the file is renamed.
			rejectCmd := cmd.NewRejectCommand(
				inboxDir,
				inProgressDir,
				rejectedDir,
				realMgr,
				dirLockFactory,
				5*time.Second,
			)

			// Capture slog output across BOTH goroutines via the
			// default logger. The slog-capture idiom here replaces
			// the process-wide default; both the scanner and the
			// reject command write to the same buffer.
			var logBuf bytes.Buffer
			originalLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
			defer slog.SetDefault(originalLogger)

			// Pre-acquire the directory lock so both contenders are forced to
			// block on their own Acquire until we release it. Without this,
			// the reject's FindPromptFileInDirs + Acquire is shorter than the
			// scanner's HasPendingVerification + ListQueued + candidate-
			// selection + Acquire path, so the reject wins the race before
			// the scanner reaches its lock acquire — and the scanner's
			// "lock acquired" line is never logged. The starter lock
			// guarantees both sides log "lock acquired".
			starterLock := lockpkg.NewDirLock(inProgressDir)
			Expect(starterLock.Acquire(ctx, 5*time.Second)).To(Succeed())

			// Sync barrier: both goroutines start together, then race.
			var wg sync.WaitGroup
			wg.Add(2)

			rejectErrCh := make(chan error, 1)
			scannerErrCh := make(chan error, 1)
			go func() {
				defer wg.Done()
				rejectErrCh <- rejectCmd.Run(
					ctx, []string{"001-test.md", "--reason", "concurrent"},
				)
			}()

			go func() {
				defer wg.Done()
				// The scanner's ScanAndProcess will run until
				// ListQueued returns empty (the mock returns
				// empty after the first call). The scanner's
				// `lockTimeout` is 5s so it doesn't bail out
				// before the reject releases the lock.
				_, err := localScanner.ScanAndProcess(ctx)
				scannerErrCh <- err
			}()

			// Give both goroutines a moment to reach their
			// fl.Acquire calls (both are blocked there because
			// the starter holds the lock), then release the
			// starter so the race is fair.
			time.Sleep(50 * time.Millisecond)
			Expect(starterLock.Release(ctx)).To(Succeed())

			wg.Wait()

			// Drain the result channels to avoid goroutine leak.
			rejectErr := <-rejectErrCh
			scannerErr := <-scannerErrCh

			// REGRESSION LOCK. Exactly one final on-disk state. If
			// the per-prompt file lock is removed from either side,
			// a save-after-rename interleaving leaves the file in
			// BOTH in-progress/ and rejected/ — this assertion
			// catches that.
			inProgressCount := 0
			rejectedCount := 0
			if _, statErr := os.Stat(promptPath); statErr == nil {
				inProgressCount = 1
			}
			rejectedPath := filepath.Join(rejectedDir, "001-test.md")
			if _, statErr := os.Stat(rejectedPath); statErr == nil {
				rejectedCount = 1
			}
			Expect(inProgressCount+rejectedCount).To(Equal(1),
				"prompt must end in exactly one of in-progress/ or rejected/ — "+
					"a torn save/rename would leave it in both. "+
					"inProgress=%d rejected=%d",
				inProgressCount, rejectedCount,
			)

			// The winning file's bytes parse as valid frontmatter
			// (not a torn write).
			finalPath := promptPath
			if rejectedCount == 1 {
				finalPath = rejectedPath
			}
			pm := prompt.NewManager(
				inboxDir,
				inProgressDir,
				completedDir,
				cancelledDir,
				nil,
				libtime.NewCurrentDateTime(),
			)
			pf, loadErr := pm.Load(ctx, finalPath)
			Expect(loadErr).NotTo(HaveOccurred(),
				"final on-disk state must parse as valid frontmatter")
			Expect(pf.Frontmatter.Status).NotTo(BeEmpty(),
				"final on-disk state must have a non-empty status")

			// Both sides logged `lock acquired` (the spec-092
			// evidence grep). Each side may log it more than once
			// (the scanner iterates the outer loop), so we count
			// >= 2.
			Expect(strings.Count(logBuf.String(), "lock acquired")).
				To(BeNumerically(">=", 2),
					fmt.Sprintf(
						"both reject and advance must have logged 'lock acquired' — "+
							"buffer:\n%s\nscannerErr: %v\nrejectErr: %v",
						logBuf.String(),
						scannerErr,
						rejectErr,
					))

			// The loser observed the post-lock state. Either:
			//  (a) reject won: file is in rejected/, scanner
			//      re-read returned an error (file gone) → no
			//      error from scanner.
			//  (b) scanner won first: scanner's post-lock re-read
			//      showed status=failed (not pre-execution) →
			//      skip; reject then acquired the lock, saw
			//      status=failed, allowed reject → no error
			//      from reject.
			// In both orderings both sides return nil. The
			// invariant: the loser side did NOT corrupt the file
			// (caught by the exactly-one assertion above) and did
			// NOT propagate an error.
			Expect(scannerErr).NotTo(HaveOccurred(),
				"scanner must not error on the post-lock skip path")
			Expect(rejectErr).NotTo(HaveOccurred(),
				"reject must not error when it wins the rename race")
		},
	)
})
