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
		ctx                context.Context
		repoNameFetcher    *mocks.RepoNameFetcher
		collaboratorLister *mocks.CollaboratorLister
	)

	BeforeEach(func() {
		ctx = context.Background()
		repoNameFetcher = new(mocks.RepoNameFetcher)
		collaboratorLister = new(mocks.CollaboratorLister)
	})

	Describe("NewCollaboratorFetcher", func() {
		It("creates a CollaboratorFetcher", func() {
			fetcher := git.NewCollaboratorFetcher(
				repoNameFetcher,
				collaboratorLister,
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
					repoNameFetcher,
					collaboratorLister,
					false,
					reviewers,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
				Expect(repoNameFetcher.FetchCallCount()).To(Equal(0))
				Expect(collaboratorLister.ListCallCount()).To(Equal(0))
			})

			It("returns provided reviewers even when useCollaborators is true", func() {
				reviewers := []string{"carol"}
				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					true,
					reviewers,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(Equal(reviewers))
				Expect(repoNameFetcher.FetchCallCount()).To(Equal(0))
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is false", func() {
			It("returns nil without calling gh CLI", func() {
				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					false,
					nil,
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
				Expect(repoNameFetcher.FetchCallCount()).To(Equal(0))
			})

			It("returns nil for empty slice", func() {
				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					false,
					[]string{},
				)
				result := fetcher.Fetch(ctx)
				Expect(result).To(BeNil())
				Expect(repoNameFetcher.FetchCallCount()).To(Equal(0))
			})
		})

		Context("when allowedReviewers is empty and useCollaborators is true", func() {
			It("fetches collaborators from GitHub", func() {
				repoNameFetcher.FetchReturns("owner/repo", nil)
				collaboratorLister.ListReturns([]string{"alice", "bob"}, nil)

				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(Equal([]string{"alice", "bob"}))
				Expect(repoNameFetcher.FetchCallCount()).To(Equal(1))
				Expect(collaboratorLister.ListCallCount()).To(Equal(1))
				_, repoName := collaboratorLister.ListArgsForCall(0)
				Expect(repoName).To(Equal("owner/repo"))
			})

			It("returns nil when repo name fetch fails", func() {
				repoNameFetcher.FetchReturns("", errors.New("no repo"))

				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(BeNil())
				Expect(collaboratorLister.ListCallCount()).To(Equal(0))
			})

			It("returns nil when collaborator list fails", func() {
				repoNameFetcher.FetchReturns("owner/repo", nil)
				collaboratorLister.ListReturns(nil, errors.New("api error"))

				fetcher := git.NewCollaboratorFetcher(
					repoNameFetcher,
					collaboratorLister,
					true,
					nil,
				)
				result := fetcher.Fetch(ctx)

				Expect(result).To(BeNil())
			})
		})
	})
})
