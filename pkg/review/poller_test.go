// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
)

var _ = Describe("ReviewPoller", func() {
	var (
		queueDir         string
		inboxDir         string
		mockFetcher      *mocks.ReviewFetcher
		mockPRMerger     *mocks.PRMerger
		mockManager      *mocks.Manager
		mockGenerator    *mocks.FixPromptGenerator
		poller           review.ReviewPoller
		allowedReviewers []string
		maxRetries       int
		promptPath       string
		prURL            string
	)

	BeforeEach(func() {
		var err error
		queueDir, err = os.MkdirTemp("", "review-poller-queue-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir, err = os.MkdirTemp("", "review-poller-inbox-*")
		Expect(err).NotTo(HaveOccurred())

		allowedReviewers = []string{"trusted-reviewer"}
		maxRetries = 3
		prURL = "https://github.com/example/repo/pull/1"

		mockFetcher = &mocks.ReviewFetcher{}
		mockPRMerger = &mocks.PRMerger{}
		mockManager = &mocks.Manager{}
		mockGenerator = &mocks.FixPromptGenerator{}

		// Create a real .md file in queueDir so os.ReadDir finds it.
		promptPath = filepath.Join(queueDir, "001-test-prompt.md")
		Expect(os.WriteFile(promptPath, []byte("# Test Prompt\n\nContent."), 0600)).To(Succeed())

		// Default: ReadFrontmatter returns in_review status.
		mockManager.ReadFrontmatterReturns(&prompt.Frontmatter{
			Status: string(prompt.InReviewPromptStatus),
			PRURL:  prURL,
		}, nil)

		// Default: Load returns a PromptFile with PRURL and Branch set.
		mockManager.LoadReturns(&prompt.PromptFile{
			Path: promptPath,
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.InReviewPromptStatus),
				PRURL:  prURL,
				Branch: "feature/test",
			},
		}, nil)

		// Default: PR is OPEN so we reach the review fetch step.
		mockFetcher.FetchPRStateReturns("OPEN", nil)

		poller = review.NewReviewPoller(
			queueDir,
			inboxDir,
			allowedReviewers,
			maxRetries,
			1*time.Millisecond,
			mockFetcher,
			mockPRMerger,
			mockManager,
			mockGenerator,
		)
	})

	AfterEach(func() {
		_ = os.RemoveAll(queueDir)
		_ = os.RemoveAll(inboxDir)
	})

	Describe("Run", func() {
		It("calls WaitAndMerge and MoveToCompleted when review is approved", func() {
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictApproved,
			}, nil)
			mockPRMerger.WaitAndMergeReturns(nil)
			mockManager.MoveToCompletedReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(func() bool {
				return mockPRMerger.WaitAndMergeCallCount() >= 1 &&
					mockManager.MoveToCompletedCallCount() >= 1
			}).Should(BeTrue())
		})

		It(
			"calls Generate and IncrementRetryCount on changes-requested within retry limit",
			func() {
				mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
					Verdict: git.ReviewVerdictChangesRequested,
					Body:    "Please fix X",
				}, nil)
				mockGenerator.GenerateReturns(nil)
				mockManager.IncrementRetryCountReturns(nil)

				runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer runCancel()
				go func() { _ = poller.Run(runCtx) }()

				Eventually(func() bool {
					return mockGenerator.GenerateCallCount() >= 1 &&
						mockManager.IncrementRetryCountCallCount() >= 1
				}).Should(BeTrue())
			},
		)

		It("verifies Generate receives correct opts on changes-requested", func() {
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Fix the error handling",
			}, nil)
			mockGenerator.GenerateReturns(nil)
			mockManager.IncrementRetryCountReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return mockGenerator.GenerateCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			_, opts := mockGenerator.GenerateArgsForCall(0)
			Expect(opts.InboxDir).To(Equal(inboxDir))
			Expect(opts.OriginalPromptName).To(Equal("001-test-prompt.md"))
			Expect(opts.Branch).To(Equal("feature/test"))
			Expect(opts.PRURL).To(Equal(prURL))
			Expect(opts.RetryCount).To(Equal(1))
			Expect(opts.ReviewBody).To(Equal("Fix the error handling"))
		})

		It("sets status to failed and does not call Generate when retry limit reached", func() {
			mockManager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Frontmatter: prompt.Frontmatter{
					Status:     string(prompt.InReviewPromptStatus),
					PRURL:      prURL,
					Branch:     "feature/test",
					RetryCount: 3, // equals maxRetries
				},
			}, nil)
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Still broken",
			}, nil)
			mockManager.SetStatusReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return mockManager.SetStatusCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			Expect(mockGenerator.GenerateCallCount()).To(Equal(0))
			_, _, status := mockManager.SetStatusArgsForCall(0)
			Expect(status).To(Equal(string(prompt.FailedPromptStatus)))
		})

		It("calls MoveToCompleted for MERGED PR without calling FetchLatestReview", func() {
			mockFetcher.FetchPRStateReturns("MERGED", nil)
			mockManager.MoveToCompletedReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return mockManager.MoveToCompletedCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			Expect(mockFetcher.FetchLatestReviewCallCount()).To(Equal(0))
		})

		It("sets status to failed for CLOSED PR", func() {
			mockFetcher.FetchPRStateReturns("CLOSED", nil)
			mockManager.SetStatusReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return mockManager.SetStatusCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			_, _, status := mockManager.SetStatusArgsForCall(0)
			Expect(status).To(Equal(string(prompt.FailedPromptStatus)))
			Expect(mockFetcher.FetchLatestReviewCallCount()).To(Equal(0))
		})

		It("takes no action when verdict is None", func() {
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictNone,
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockPRMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))
			Expect(mockGenerator.GenerateCallCount()).To(Equal(0))
		})

		It("logs warning and continues when FetchPRState returns error", func() {
			mockFetcher.FetchPRStateReturns("", stderrors.New("network error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockPRMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("logs warning and continues when FetchLatestReview returns error", func() {
			mockFetcher.FetchLatestReviewReturns(nil, stderrors.New("api error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockPRMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("skips prompt and logs warning when PRURL is empty", func() {
			mockManager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Frontmatter: prompt.Frontmatter{
					Status: string(prompt.InReviewPromptStatus),
					PRURL:  "",
				},
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockFetcher.FetchPRStateCallCount()).To(Equal(0))
		})

		It("returns nil when context is cancelled", func() {
			mockManager.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status: string(prompt.ApprovedPromptStatus),
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())
		})

		It("logs warning and continues when WaitAndMerge fails on approval", func() {
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictApproved,
			}, nil)
			mockPRMerger.WaitAndMergeReturns(stderrors.New("merge failed"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockPRMerger.WaitAndMergeCallCount()).To(BeNumerically(">=", 1))
			// MoveToCompleted must NOT be called since WaitAndMerge failed.
			Expect(mockManager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("logs warning and continues when Generate fails on changes-requested", func() {
			mockFetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Fix it",
			}, nil)
			mockGenerator.GenerateReturns(stderrors.New("write error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockGenerator.GenerateCallCount()).To(BeNumerically(">=", 1))
			// IncrementRetryCount must NOT be called if Generate failed.
			Expect(mockManager.IncrementRetryCountCallCount()).To(Equal(0))
		})

		It("logs warning but continues when queueDir cannot be read", func() {
			// Remove queueDir so os.ReadDir fails.
			Expect(os.RemoveAll(queueDir)).To(Succeed())

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())
		})

		It("skips file and continues when ReadFrontmatter returns error", func() {
			mockManager.ReadFrontmatterReturns(nil, stderrors.New("parse error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(mockFetcher.FetchPRStateCallCount()).To(Equal(0))
		})
	})
})
