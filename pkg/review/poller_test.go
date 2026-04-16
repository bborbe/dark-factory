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
	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/review"
)

var _ = Describe("ReviewPoller", func() {
	var (
		queueDir            string
		inboxDir            string
		fetcher             *mocks.ReviewFetcher
		prMerger            *mocks.PRMerger
		manager             *mocks.ReviewPromptManager
		generator           *mocks.FixPromptGenerator
		collaboratorFetcher *mocks.CollaboratorFetcher
		poller              review.ReviewPoller
		maxRetries          int
		promptPath          string
		prURL               string
	)

	BeforeEach(func() {
		var err error
		queueDir, err = os.MkdirTemp("", "review-poller-queue-*")
		Expect(err).NotTo(HaveOccurred())

		inboxDir, err = os.MkdirTemp("", "review-poller-inbox-*")
		Expect(err).NotTo(HaveOccurred())

		maxRetries = 3
		prURL = "https://github.com/example/repo/pull/1"

		fetcher = &mocks.ReviewFetcher{}
		prMerger = &mocks.PRMerger{}
		manager = &mocks.ReviewPromptManager{}
		generator = &mocks.FixPromptGenerator{}
		collaboratorFetcher = &mocks.CollaboratorFetcher{}
		collaboratorFetcher.FetchReturns([]string{"trusted-reviewer"})

		// Create a real .md file in queueDir so os.ReadDir finds it.
		promptPath = filepath.Join(queueDir, "001-test-prompt.md")
		Expect(os.WriteFile(promptPath, []byte("# Test Prompt\n\nContent."), 0600)).To(Succeed())

		// Default: ReadFrontmatter returns in_review status.
		manager.ReadFrontmatterReturns(&prompt.Frontmatter{
			Status: string(prompt.InReviewPromptStatus),
			PRURL:  prURL,
		}, nil)

		// Default: Load returns a PromptFile with PRURL and Branch set.
		manager.LoadReturns(&prompt.PromptFile{
			Path: promptPath,
			Frontmatter: prompt.Frontmatter{
				Status: string(prompt.InReviewPromptStatus),
				PRURL:  prURL,
				Branch: "feature/test",
			},
		}, nil)

		// Default: PR is OPEN so we reach the review fetch step.
		fetcher.FetchPRStateReturns("OPEN", nil)

		poller = review.NewReviewPoller(
			queueDir,
			inboxDir,
			collaboratorFetcher,
			maxRetries,
			1*time.Millisecond,
			fetcher,
			prMerger,
			manager,
			generator,
			"",
			notifier.NewMultiNotifier(),
		)
	})

	AfterEach(func() {
		_ = os.RemoveAll(queueDir)
		_ = os.RemoveAll(inboxDir)
	})

	Describe("Run", func() {
		It("calls WaitAndMerge and MoveToCompleted when review is approved", func() {
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictApproved,
			}, nil)
			prMerger.WaitAndMergeReturns(nil)
			manager.MoveToCompletedReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(func() bool {
				return prMerger.WaitAndMergeCallCount() >= 1 &&
					manager.MoveToCompletedCallCount() >= 1
			}).Should(BeTrue())
		})

		It(
			"calls Generate and IncrementRetryCount on changes-requested within retry limit",
			func() {
				fetcher.FetchLatestReviewReturns(&git.ReviewResult{
					Verdict: git.ReviewVerdictChangesRequested,
					Body:    "Please fix X",
				}, nil)
				generator.GenerateReturns(nil)
				manager.IncrementRetryCountReturns(nil)

				runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer runCancel()
				go func() { _ = poller.Run(runCtx) }()

				Eventually(func() bool {
					return generator.GenerateCallCount() >= 1 &&
						manager.IncrementRetryCountCallCount() >= 1
				}).Should(BeTrue())
			},
		)

		It("verifies Generate receives correct opts on changes-requested", func() {
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Fix the error handling",
			}, nil)
			generator.GenerateReturns(nil)
			manager.IncrementRetryCountReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return generator.GenerateCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			_, opts := generator.GenerateArgsForCall(0)
			Expect(opts.InboxDir).To(Equal(inboxDir))
			Expect(opts.OriginalPromptName).To(Equal("001-test-prompt.md"))
			Expect(opts.Branch).To(Equal("feature/test"))
			Expect(opts.PRURL).To(Equal(prURL))
			Expect(opts.RetryCount).To(Equal(1))
			Expect(opts.ReviewBody).To(Equal("Fix the error handling"))
		})

		It("sets status to failed and does not call Generate when retry limit reached", func() {
			manager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Frontmatter: prompt.Frontmatter{
					Status:     string(prompt.InReviewPromptStatus),
					PRURL:      prURL,
					Branch:     "feature/test",
					RetryCount: 3, // equals maxRetries
				},
			}, nil)
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Still broken",
			}, nil)
			manager.SetStatusReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return manager.SetStatusCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			Expect(generator.GenerateCallCount()).To(Equal(0))
			_, _, status := manager.SetStatusArgsForCall(0)
			Expect(status).To(Equal(string(prompt.FailedPromptStatus)))
		})

		It("fires review_limit notification when retry limit is reached", func() {
			fakeNotifier := &mocks.Notifier{}

			pollerWithNotifier := review.NewReviewPoller(
				queueDir,
				inboxDir,
				collaboratorFetcher,
				maxRetries,
				1*time.Millisecond,
				fetcher,
				prMerger,
				manager,
				generator,
				"test-project",
				fakeNotifier,
			)

			manager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Frontmatter: prompt.Frontmatter{
					Status:     string(prompt.InReviewPromptStatus),
					PRURL:      prURL,
					Branch:     "feature/test",
					RetryCount: 3, // equals maxRetries
				},
			}, nil)
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Still broken",
			}, nil)
			manager.SetStatusReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = pollerWithNotifier.Run(runCtx) }()

			Eventually(func() int { return fakeNotifier.NotifyCallCount() }).
				Should(BeNumerically(">=", 1))
			runCancel()

			_, event := fakeNotifier.NotifyArgsForCall(0)
			Expect(event.EventType).To(Equal("review_limit"))
			Expect(event.ProjectName).To(Equal("test-project"))
			Expect(event.PRURL).To(Equal(prURL))
		})

		It("calls MoveToCompleted for MERGED PR without calling FetchLatestReview", func() {
			fetcher.FetchPRStateReturns("MERGED", nil)
			manager.MoveToCompletedReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return manager.MoveToCompletedCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			Expect(fetcher.FetchLatestReviewCallCount()).To(Equal(0))
		})

		It("sets status to failed for CLOSED PR", func() {
			fetcher.FetchPRStateReturns("CLOSED", nil)
			manager.SetStatusReturns(nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer runCancel()
			go func() { _ = poller.Run(runCtx) }()

			Eventually(
				func() int { return manager.SetStatusCallCount() },
			).Should(BeNumerically(">=", 1))
			runCancel()

			_, _, status := manager.SetStatusArgsForCall(0)
			Expect(status).To(Equal(string(prompt.FailedPromptStatus)))
			Expect(fetcher.FetchLatestReviewCallCount()).To(Equal(0))
		})

		It("takes no action when verdict is None", func() {
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictNone,
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(prMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))
			Expect(generator.GenerateCallCount()).To(Equal(0))
		})

		It("logs warning and continues when FetchPRState returns error", func() {
			fetcher.FetchPRStateReturns("", stderrors.New("network error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(prMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("logs warning and continues when FetchLatestReview returns error", func() {
			fetcher.FetchLatestReviewReturns(nil, stderrors.New("api error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(prMerger.WaitAndMergeCallCount()).To(Equal(0))
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("skips prompt and logs warning when PRURL is empty", func() {
			manager.LoadReturns(&prompt.PromptFile{
				Path: promptPath,
				Frontmatter: prompt.Frontmatter{
					Status: string(prompt.InReviewPromptStatus),
					PRURL:  "",
				},
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(fetcher.FetchPRStateCallCount()).To(Equal(0))
		})

		It("returns nil when context is cancelled", func() {
			manager.ReadFrontmatterReturns(&prompt.Frontmatter{
				Status: string(prompt.ApprovedPromptStatus),
			}, nil)

			runCtx, runCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())
		})

		It("logs warning and continues when WaitAndMerge fails on approval", func() {
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictApproved,
			}, nil)
			prMerger.WaitAndMergeReturns(stderrors.New("merge failed"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(prMerger.WaitAndMergeCallCount()).To(BeNumerically(">=", 1))
			// MoveToCompleted must NOT be called since WaitAndMerge failed.
			Expect(manager.MoveToCompletedCallCount()).To(Equal(0))
		})

		It("logs warning and continues when Generate fails on changes-requested", func() {
			fetcher.FetchLatestReviewReturns(&git.ReviewResult{
				Verdict: git.ReviewVerdictChangesRequested,
				Body:    "Fix it",
			}, nil)
			generator.GenerateReturns(stderrors.New("write error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(generator.GenerateCallCount()).To(BeNumerically(">=", 1))
			// IncrementRetryCount must NOT be called if Generate failed.
			Expect(manager.IncrementRetryCountCallCount()).To(Equal(0))
		})

		It("logs warning but continues when queueDir cannot be read", func() {
			// Remove queueDir so os.ReadDir fails.
			Expect(os.RemoveAll(queueDir)).To(Succeed())

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())
		})

		It("skips file and continues when ReadFrontmatter returns error", func() {
			manager.ReadFrontmatterReturns(nil, stderrors.New("parse error"))

			runCtx, runCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer runCancel()
			Expect(poller.Run(runCtx)).To(Succeed())

			Expect(fetcher.FetchPRStateCallCount()).To(Equal(0))
		})
	})
})
