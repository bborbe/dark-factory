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
	client        *bitbucketClient
	project       string
	repo          string
	defaultBranch string
	currentUser   string
}

// NewBitbucketCollaboratorFetcher creates a CollaboratorFetcher backed by the Bitbucket Server
// default-reviewers plugin. currentUser is excluded from the result to avoid self-review.
func NewBitbucketCollaboratorFetcher(
	baseURL string,
	token string,
	project string,
	repo string,
	defaultBranch string,
	currentUser string,
) CollaboratorFetcher {
	return &bitbucketCollaboratorFetcher{
		client:        newBitbucketClient(baseURL, token),
		project:       project,
		repo:          repo,
		defaultBranch: defaultBranch,
		currentUser:   currentUser,
	}
}

type bbDefaultReviewersResponse struct {
	Errors    []interface{} `json:"errors"`
	Reviewers []struct {
		Slug string `json:"slug"`
	} `json:"reviewers"`
}

// Fetch returns the list of default reviewers from the Bitbucket Server default-reviewers plugin.
// Returns nil on error (best-effort, graceful degradation).
func (b *bitbucketCollaboratorFetcher) Fetch(ctx context.Context) []string {
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

	var reviewers []string
	for _, r := range result.Reviewers {
		if r.Slug != b.currentUser {
			reviewers = append(reviewers, r.Slug)
		}
	}
	return reviewers
}
