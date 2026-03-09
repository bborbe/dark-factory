// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("CollaboratorFetcher", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewCollaboratorFetcher", func() {
		It("creates a CollaboratorFetcher", func() {
			fetcher := git.NewCollaboratorFetcher("", false, nil)
			Expect(fetcher).NotTo(BeNil())
		})
	})

	Describe("Fetch", func() {
		Context("when allowedReviewers is non-empty", func() {
			It("returns the provided reviewers without calling gh CLI", func() {
				reviewers := []string{"alice", "bob"}
				fetcher := git.NewCollaboratorFetcher("", false, reviewers)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
			})

			It("returns provided reviewers even when useCollaborators is true", func() {
				reviewers := []string{"carol"}
				fetcher := git.NewCollaboratorFetcher("token", true, reviewers)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is false", func() {
			It("returns nil without calling gh CLI", func() {
				fetcher := git.NewCollaboratorFetcher("", false, nil)
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
			})

			It("returns nil for empty slice", func() {
				fetcher := git.NewCollaboratorFetcher("token", false, []string{})
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is true", func() {
			It("returns nil when gh CLI fails (no GitHub context)", func() {
				fetcher := git.NewCollaboratorFetcher("", true, nil)
				result := fetcher.Fetch(ctx)
				// gh CLI will fail in test environment — gracefully returns nil
				Expect(result).To(BeNil())
			})

			It("returns nil when gh CLI fails with a non-empty token", func() {
				fetcher := git.NewCollaboratorFetcher("some-token", true, nil)
				result := fetcher.Fetch(ctx)
				// gh CLI will fail in test environment — gracefully returns nil
				Expect(result).To(BeNil())
			})
		})
	})
})
