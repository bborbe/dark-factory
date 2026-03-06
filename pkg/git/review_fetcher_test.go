// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseReviews", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns approved verdict when trusted reviewer approved", func() {
		output := []byte(`{"state":"APPROVED","author":"alice","body":"LGTM"}`)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictApproved))
		Expect(result.Body).To(Equal("LGTM"))
	})

	It("returns changes_requested when trusted reviewer requested changes", func() {
		output := []byte(`{"state":"CHANGES_REQUESTED","author":"alice","body":"needs work"}`)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictChangesRequested))
		Expect(result.Body).To(Equal("needs work"))
	})

	It("returns none when only untrusted reviewers", func() {
		output := []byte(`{"state":"APPROVED","author":"mallory","body":"approve"}`)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictNone))
	})

	It("returns none when no reviews", func() {
		result, err := parseReviews(ctx, []byte(""), []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictNone))
	})

	It("returns none for unknown review state from trusted reviewer", func() {
		output := []byte(`{"state":"COMMENTED","author":"alice","body":"see inline"}`)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictNone))
		Expect(result.Body).To(Equal("see inline"))
	})

	It("takes the last trusted review when multiple reviews exist", func() {
		output := []byte("" +
			`{"state":"CHANGES_REQUESTED","author":"alice","body":"first review"}` + "\n" +
			`{"state":"APPROVED","author":"alice","body":"second review"}`,
		)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictApproved))
		Expect(result.Body).To(Equal("second review"))
	})

	It("ignores untrusted reviews between trusted ones", func() {
		output := []byte("" +
			`{"state":"APPROVED","author":"alice","body":"trusted approve"}` + "\n" +
			`{"state":"CHANGES_REQUESTED","author":"mallory","body":"untrusted"}`,
		)
		result, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(ReviewVerdictApproved))
	})

	It("returns error on invalid json", func() {
		output := []byte(`not-json`)
		_, err := parseReviews(ctx, output, []string{"alice"})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("prMerger mergePR", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns error when context is cancelled (no token)", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		p := &prMerger{ghToken: "", pollInterval: time.Second, mergeTimeout: time.Minute}
		err := p.mergePR(cancelCtx, "https://github.com/owner/repo/pull/1")
		Expect(err).To(HaveOccurred())
	})

	It("returns error when context is cancelled (with token)", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		p := &prMerger{ghToken: "mytoken", pollInterval: time.Second, mergeTimeout: time.Minute}
		err := p.mergePR(cancelCtx, "https://github.com/owner/repo/pull/1")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("prMerger checkPRStatus", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns error when context is cancelled (no token)", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		p := &prMerger{ghToken: "", pollInterval: time.Second, mergeTimeout: time.Minute}
		_, err := p.checkPRStatus(cancelCtx, "https://github.com/owner/repo/pull/1")
		Expect(err).To(HaveOccurred())
	})

	It("returns error when context is cancelled (with token)", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		p := &prMerger{ghToken: "mytoken", pollInterval: time.Second, mergeTimeout: time.Minute}
		_, err := p.checkPRStatus(cancelCtx, "https://github.com/owner/repo/pull/1")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ReviewFetcher", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("FetchLatestReview", func() {
		It("returns error when context is cancelled (no token)", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			fetcher := NewReviewFetcher("")
			_, err := fetcher.FetchLatestReview(
				cancelCtx,
				"https://github.com/owner/repo/pull/1",
				[]string{"alice"},
			)
			Expect(err).To(HaveOccurred())
		})

		It("returns error when context is cancelled (with token)", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			fetcher := NewReviewFetcher("ghp_token")
			_, err := fetcher.FetchLatestReview(
				cancelCtx,
				"https://github.com/owner/repo/pull/1",
				[]string{"alice"},
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("FetchPRState", func() {
		It("returns error when context is cancelled (no token)", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			fetcher := NewReviewFetcher("")
			_, err := fetcher.FetchPRState(cancelCtx, "https://github.com/owner/repo/pull/1")
			Expect(err).To(HaveOccurred())
		})

		It("returns error when context is cancelled (with token)", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			fetcher := NewReviewFetcher("ghp_token")
			_, err := fetcher.FetchPRState(cancelCtx, "https://github.com/owner/repo/pull/1")
			Expect(err).To(HaveOccurred())
		})
	})
})
