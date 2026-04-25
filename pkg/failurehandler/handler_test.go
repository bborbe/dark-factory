// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package failurehandler_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/failurehandler"
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
)

var _ = Describe("Handler", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		tempDir      string
		completedDir string
		promptPath   string
		promptMgr    *mocks.FailureHandlerPromptManager
		n            *mocks.Notifier
		h            failurehandler.Handler
	)

	BeforeEach(func() {
		var err error
		ctx, cancel = context.WithCancel(context.Background())
		tempDir, err = os.MkdirTemp("", "failurehandler-test-*")
		Expect(err).NotTo(HaveOccurred())
		completedDir = filepath.Join(tempDir, "completed")
		err = os.MkdirAll(completedDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		queueDir := filepath.Join(tempDir, "queue")
		err = os.MkdirAll(queueDir, 0750)
		Expect(err).NotTo(HaveOccurred())
		promptPath = filepath.Join(queueDir, "001-my-prompt.md")
		err = os.WriteFile(promptPath, []byte("---\nstatus: approved\n---\n# My prompt\n"), 0600)
		Expect(err).NotTo(HaveOccurred())

		promptMgr = &mocks.FailureHandlerPromptManager{}
		n = &mocks.Notifier{}
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	// makePromptFile creates a PromptFile at promptPath on disk and also returns it for stubbing.
	makePromptFile := func(retryCount int) *prompt.PromptFile {
		pf := prompt.NewPromptFile(
			promptPath,
			prompt.Frontmatter{
				Status:     "approved",
				RetryCount: retryCount,
			},
			[]byte("# My prompt\n"),
			libtime.NewCurrentDateTime(),
		)
		return pf
	}

	Describe("Handle", func() {
		Context("when ctx is already cancelled", func() {
			BeforeEach(func() {
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 0)
			})

			It("returns a wrapped error and does not touch the prompt", func() {
				cancel()
				err := h.Handle(ctx, promptPath, stderrors.New("container exit 1"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("prompt failed"))
				Expect(promptMgr.LoadCallCount()).To(Equal(0))
			})
		})

		Context("when the prompt file has moved to completed (post-execution failure)", func() {
			BeforeEach(func() {
				// Remove from queue, place in completed
				Expect(os.Remove(promptPath)).To(Succeed())
				err := os.WriteFile(
					filepath.Join(completedDir, filepath.Base(promptPath)),
					[]byte("---\nstatus: completed\n---\n"),
					0600,
				)
				Expect(err).NotTo(HaveOccurred())
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 0)
			})

			It("returns a stop error", func() {
				err := h.Handle(ctx, promptPath, stderrors.New("git push failed"))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("post-execution git failure"))
			})
		})

		Context("with autoRetryLimit > 0 and retries available", func() {
			BeforeEach(func() {
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 3)
				pf := makePromptFile(0)
				promptMgr.LoadReturns(pf, nil)
			})

			It("increments retry count, marks approved, saves, and returns nil", func() {
				err := h.Handle(ctx, promptPath, stderrors.New("some error"))
				Expect(err).NotTo(HaveOccurred())

				Expect(promptMgr.LoadCallCount()).To(Equal(1))
				// The PromptFile was saved (written to disk) — file should have approved status
				saved, readErr := prompt.Load(ctx, promptPath, libtime.NewCurrentDateTime())
				Expect(readErr).NotTo(HaveOccurred())
				Expect(saved.Frontmatter.Status).To(Equal("approved"))
				Expect(saved.Frontmatter.RetryCount).To(Equal(1))
				// No failure notification
				Expect(n.NotifyCallCount()).To(Equal(0))
			})
		})

		Context("with autoRetryLimit == 0 (disabled)", func() {
			BeforeEach(func() {
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 0)
				pf := makePromptFile(0)
				promptMgr.LoadReturns(pf, nil)
				n.NotifyReturns(nil)
			})

			It("marks failed, saves, notifies, and returns nil", func() {
				err := h.Handle(ctx, promptPath, stderrors.New("fatal error"))
				Expect(err).NotTo(HaveOccurred())

				saved, readErr := prompt.Load(ctx, promptPath, libtime.NewCurrentDateTime())
				Expect(readErr).NotTo(HaveOccurred())
				Expect(saved.Frontmatter.Status).To(Equal("failed"))

				Expect(n.NotifyCallCount()).To(Equal(1))
				_, evt := n.NotifyArgsForCall(0)
				Expect(evt.EventType).To(Equal("prompt_failed"))
				Expect(evt.ProjectName).To(Equal("test-project"))
				Expect(evt.PromptName).To(Equal(filepath.Base(promptPath)))
			})
		})

		Context("when retries are exhausted", func() {
			BeforeEach(func() {
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 2)
				// retryCount already at limit
				pf := makePromptFile(2)
				promptMgr.LoadReturns(pf, nil)
				n.NotifyReturns(nil)
			})

			It("marks failed, notifies, and returns nil", func() {
				err := h.Handle(ctx, promptPath, stderrors.New("error after retries"))
				Expect(err).NotTo(HaveOccurred())

				saved, readErr := prompt.Load(ctx, promptPath, libtime.NewCurrentDateTime())
				Expect(readErr).NotTo(HaveOccurred())
				Expect(saved.Frontmatter.Status).To(Equal("failed"))

				Expect(n.NotifyCallCount()).To(Equal(1))
			})
		})

		Context("when Load fails", func() {
			BeforeEach(func() {
				h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 0)
				promptMgr.LoadReturns(nil, stderrors.New("load error"))
			})

			It("logs and returns nil without panicking", func() {
				Expect(func() {
					err := h.Handle(ctx, promptPath, stderrors.New("some err"))
					Expect(err).NotTo(HaveOccurred())
				}).NotTo(Panic())
			})
		})
	})

	Describe("NotifyFromReport", func() {
		BeforeEach(func() {
			h = failurehandler.NewHandler(promptMgr, n, completedDir, "test-project", 0)
		})

		Context("when log file does not exist", func() {
			It("is a no-op", func() {
				h.NotifyFromReport(ctx, "/nonexistent/log.txt", promptPath)
				Expect(n.NotifyCallCount()).To(Equal(0))
			})
		})

		Context("when log file has no completion report", func() {
			It("is a no-op", func() {
				logFile := filepath.Join(tempDir, "no-report.log")
				Expect(os.WriteFile(logFile, []byte("some random output\n"), 0600)).To(Succeed())
				h.NotifyFromReport(ctx, logFile, promptPath)
				Expect(n.NotifyCallCount()).To(Equal(0))
			})
		})

		Context("when log file has a partial report", func() {
			It("fires a prompt_partial notification", func() {
				logFile := filepath.Join(tempDir, "partial.log")
				cr := report.CompletionReport{
					Status:  "partial",
					Summary: "things mostly worked",
				}
				writeReportToLog(logFile, cr)
				n.NotifyReturns(nil)
				h.NotifyFromReport(ctx, logFile, promptPath)
				Expect(n.NotifyCallCount()).To(Equal(1))
				_, evt := n.NotifyArgsForCall(0)
				Expect(evt.EventType).To(Equal("prompt_partial"))
				Expect(evt.ProjectName).To(Equal("test-project"))
				Expect(evt.PromptName).To(Equal(filepath.Base(promptPath)))
			})
		})

		Context("when log file has a success report", func() {
			It("does not notify", func() {
				logFile := filepath.Join(tempDir, "success.log")
				cr := report.CompletionReport{Status: "success", Summary: "all good"}
				writeReportToLog(logFile, cr)
				h.NotifyFromReport(ctx, logFile, promptPath)
				Expect(n.NotifyCallCount()).To(Equal(0))
			})
		})
	})

	Describe("NewHandler satisfies Handler interface", func() {
		It("compiles with Handler interface", func() {
			// Compile-time check: NewHandler returns a Handler.
			h2 := failurehandler.NewHandler(
				&mocks.FailureHandlerPromptManager{},
				notifier.NewMultiNotifier(),
				completedDir,
				"proj",
				0,
			)
			Expect(h2).NotTo(BeNil())
		})
	})
})

// writeReportToLog writes a JSON completion report block to a log file, matching the format
// that report.ParseFromLog expects (MarkerStart...JSON...MarkerEnd).
func writeReportToLog(logFile string, cr report.CompletionReport) {
	jsonStr := `{"status":"` + cr.Status + `","summary":"` + cr.Summary + `","blockers":[]}`
	content := "some preceding log output\n\n" +
		"<!-- DARK-FACTORY-REPORT\n" +
		jsonStr + "\n" +
		"DARK-FACTORY-REPORT -->\n"
	Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())
}
