// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/dark-factory/mocks"
	"github.com/bborbe/dark-factory/pkg/git"
)

var _ = Describe("CollaboratorFetcher", func() {
	var (
		ctx                    context.Context
		mockRepoNameFetcher    *mocks.RepoNameFetcher
		mockCollaboratorLister *mocks.CollaboratorLister
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockRepoNameFetcher = new(mocks.RepoNameFetcher)
		mockCollaboratorLister = new(mocks.CollaboratorLister)
	})

	Describe("NewCollaboratorFetcher", func() {
		It("creates a CollaboratorFetcher", func() {
			fetcher := git.NewCollaboratorFetcher(
				mockRepoNameFetcher,
				mockCollaboratorLister,
				false,
				nil,
			)
			Expect(fetcher).NotTo(BeNil())
		})
	})

	Describe("Fetch", func() {
		Context("when allowedReviewers is non-empty", func() {
			It("returns the provided reviewers without calling gh CLI", func() {
				reviewers := []string{"alice", "bob"}
				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					false,
					reviewers,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
				Expect(mockRepoNameFetcher.FetchCallCount()).To(Equal(0))
				Expect(mockCollaboratorLister.ListCallCount()).To(Equal(0))
			})

			It("returns provided reviewers even when useCollaborators is true", func() {
				reviewers := []string{"carol"}
				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					true,
					reviewers,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
				Expect(mockRepoNameFetcher.FetchCallCount()).To(Equal(0))
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is false", func() {
			It("returns nil without calling gh CLI", func() {
				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					false,
					nil,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
				Expect(mockRepoNameFetcher.FetchCallCount()).To(Equal(0))
			})

			It("returns nil for empty slice", func() {
				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					false,
					[]string{},
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
				Expect(mockRepoNameFetcher.FetchCallCount()).To(Equal(0))
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is true", func() {
			It("fetches collaborators from GitHub", func() {
				mockRepoNameFetcher.FetchReturns("owner/repo", nil)
				mockCollaboratorLister.ListReturns([]string{"alice", "bob"}, nil)

				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(Equal([]string{"alice", "bob"}))
				Expect(mockRepoNameFetcher.FetchCallCount()).To(Equal(1))
				Expect(mockCollaboratorLister.ListCallCount()).To(Equal(1))
				_, repoName := mockCollaboratorLister.ListArgsForCall(0)
				Expect(repoName).To(Equal("owner/repo"))
			})

			It("returns nil when repo name fetch fails", func() {
				mockRepoNameFetcher.FetchReturns("", errors.New("no repo"))

				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(BeNil())
				Expect(mockCollaboratorLister.ListCallCount()).To(Equal(0))
			})

			It("returns nil when collaborator list fails", func() {
				mockRepoNameFetcher.FetchReturns("owner/repo", nil)
				mockCollaboratorLister.ListReturns(nil, errors.New("api error"))

				fetcher := git.NewCollaboratorFetcher(
					mockRepoNameFetcher,
					mockCollaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(BeNil())
			})
		})
	})
})
