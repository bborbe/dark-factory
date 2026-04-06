// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"fmt"
	"log/slog"
)

// bitbucketCollaboratorFetcher implements CollaboratorFetcher for Bitbucket Server.
// It fetches default reviewers from the Bitbucket Server default-reviewers plugin.
type bitbucketCollaboratorFetcher struct {
	client             *bitbucketClient
	project            string
	repo               string
	defaultBranch      string
	currentUserFetcher BitbucketCurrentUserFetcher
	allowedReviewers   []string
}

// NewBitbucketCollaboratorFetcher creates a CollaboratorFetcher backed by the Bitbucket Server
// default-reviewers plugin. currentUserFetcher is called lazily to exclude the current user from results.
// If allowedReviewers is non-empty, it is returned directly without any HTTP calls.
func NewBitbucketCollaboratorFetcher(
	baseURL string,
	token string,
	project string,
	repo string,
	defaultBranch string,
	currentUserFetcher BitbucketCurrentUserFetcher,
	allowedReviewers []string,
) CollaboratorFetcher {
	return &bitbucketCollaboratorFetcher{
		client:             newBitbucketClient(baseURL, token),
		project:            project,
		repo:               repo,
		defaultBranch:      defaultBranch,
		currentUserFetcher: currentUserFetcher,
		allowedReviewers:   allowedReviewers,
	}
}

type bbDefaultReviewersResponse struct {
	Errors    []interface{} `json:"errors"`
	Reviewers []struct {
		Slug string `json:"slug"`
	} `json:"reviewers"`
}

// Fetch returns the list of default reviewers from the Bitbucket Server default-reviewers plugin.
// If allowedReviewers was provided at construction time, those are returned directly.
// Returns nil on error (best-effort, graceful degradation).
func (b *bitbucketCollaboratorFetcher) Fetch(ctx context.Context) []string {
	if len(b.allowedReviewers) > 0 {
		return b.allowedReviewers
	}

	targetBranch := b.defaultBranch
	if targetBranch == "" {
		targetBranch = "master"
	}

	path := fmt.Sprintf(
		"/rest/default-reviewers/1.0/projects/%s/repos/%s/reviewers?sourceRepoId=&targetRepoId=&sourceRefId=refs/heads/feature&targetRefId=refs/heads/%s",
		b.project,
		b.repo,
		targetBranch,
	)

	var result bbDefaultReviewersResponse
	if err := b.client.do(ctx, "GET", path, nil, &result); err != nil {
		slog.Warn(
			"bitbucket: default-reviewers plugin unavailable or returned error — PR will be created without reviewers",
			"error",
			err,
		)
		return nil
	}

	currentUser := b.currentUserFetcher.FetchCurrentUser(ctx)
	var reviewers []string
	for _, r := range result.Reviewers {
		if r.Slug != currentUser {
			reviewers = append(reviewers, r.Slug)
		}
	}
	return reviewers
}
