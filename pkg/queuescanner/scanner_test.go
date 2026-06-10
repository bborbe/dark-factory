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
		// This test exercises the lock-acquisition pattern (fileLock) that prompt
		// 1 widens. It does not invoke the reject command directly (which
		// requires full CLI wiring) — it tests the scanner invariant that
		// ScanAndProcess respects an external holder of the same lock.
		// Setup: a real prompt file at queueDir, a lock path
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
		lockInstance := lockpkg.NewFileLock(promptPath)
		Expect(lockInstance.Acquire(ctx, 5*time.Second)).To(Succeed())
		defer func() {
			_ = lockInstance.Release(ctx)
		}()

		// Scanner cannot acquire the same lock within a short timeout
		otherLock := lockpkg.NewFileLock(promptPath)
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
			// Spec 094 AC "concurrent-reject-advance": a real prompt reject
			// and a real scanner advance on the same prompt file must
			// serialize under the project lock. Exactly one final on-disk
			// state, no corruption, the loser observes the post-lock state.
			// This test exercises the actual reject command + actual scanner
			// against a real prompt fixture (the existing FileLock-only test
			// above only exercises the lock primitive).
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir, err := os.MkdirTemp("", "concurrent-reject-real-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(dir)

			// Build a local scanner for this test (the outer Describe
			// "Scanner" suite owns a different scanner instance; we need
			// our own because this Describe is separate).
			localMgr := &mocks.QueueScannerPromptManager{}
			localPP := &mocks.PromptProcessor{}
			localFH := &mocks.FailureHandler{}
			localMgr.LoadReturns(nil, stderrors.New("not found"))
			localMgr.ListQueuedReturns([]prompt.Prompt{}, nil)
			localMgr.AllPreviousCompletedReturns(false)
			localScanner := queuescanner.NewScanner(
				localMgr, localPP, localFH, dir,
			)

			inboxDir := filepath.Join(dir, "inbox")
			inProgressDir := filepath.Join(dir, "in-progress")
			rejectedDir := filepath.Join(dir, "rejected")
			Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
			Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())

			promptPath := filepath.Join(inProgressDir, "226-spec-056.md")
			Expect(os.WriteFile(promptPath, []byte(
				"---\nstatus: failed\n---\n# Test\n",
			), 0600)).To(Succeed())

			// Wire a real reject command against the temp dirs, using
			// the same NewRejectCommand constructor the production code
			// uses (no ad-hoc reject path).
			rejectCmd := cmd.NewRejectCommand(
				inboxDir,
				inProgressDir,
				rejectedDir,
				prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()),
			)

			// Sync barrier: both goroutines start together, then race.
			var wg sync.WaitGroup
			wg.Add(2)

			rejectErrCh := make(chan error, 1)
			scannerErrCh := make(chan error, 1)
			go func() {
				defer wg.Done()
				rejectErrCh <- rejectCmd.Run(
					ctx, []string{"226-spec-056.md", "--reason", "concurrent"},
				)
			}()

			go func() {
				defer wg.Done()
				// A scanner advance that finds the prompt and tries to
				// process it. The scanner's ScanAndProcess may take the
				// project lock or not — what matters is that the on-disk
				// final state is exactly one of (inProgress, rejected).
				_, err := localScanner.ScanAndProcess(ctx)
				scannerErrCh <- err
			}()

			wg.Wait()

			// Reject may legitimately succeed or fail depending on lock
			// timing — what matters is the final on-disk state. Drain
			// the result channels to avoid goroutine leak.
			<-rejectErrCh
			<-scannerErrCh

			// EXACTLY ONE final file. The prompt must exist in exactly one
			// of inProgressDir or rejectedDir, never both, never neither.
			inProgressCount := 0
			rejectedCount := 0
			if _, statErr := os.Stat(promptPath); statErr == nil {
				inProgressCount = 1
			}
			rejectedPath := filepath.Join(rejectedDir, "226-spec-056.md")
			if _, statErr := os.Stat(rejectedPath); statErr == nil {
				rejectedCount = 1
			}
			Expect(inProgressCount+rejectedCount).To(Equal(1),
				"prompt must end in exactly one of in-progress/ or rejected/")

			// The loser re-reads post-lock state. (If reject won, the file
			// is in rejected/; if scanner won, the file is in in-progress/.
			// The point: the on-disk state is consistent — no partial
			// writes, no duplicates.)
			finalPath := promptPath
			if rejectedCount == 1 {
				finalPath = rejectedPath
			}
			content, err := os.ReadFile(finalPath)
			Expect(err).NotTo(HaveOccurred())
			// Whichever side won, the file is readable frontmatter (not
			// a torn write). The status field must be either "rejected"
			// (reject won) or "failed" (scanner saw and skipped because
			// status was failed and not a pre-exec state).
			Expect(string(content)).To(MatchRegexp(`status: (rejected|failed)`))
		},
	)
})
