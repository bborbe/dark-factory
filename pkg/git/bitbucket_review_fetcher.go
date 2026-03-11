// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"fmt"

	"github.com/bborbe/errors"
)

// bitbucketReviewFetcher implements ReviewFetcher for Bitbucket Server.
type bitbucketReviewFetcher struct {
	client  *bitbucketClient
	project string
	repo    string
}

// NewBitbucketReviewFetcher creates a ReviewFetcher backed by the Bitbucket Server REST API.
func NewBitbucketReviewFetcher(
	baseURL string,
	token string,
	project string,
	repo string,
) ReviewFetcher {
	return &bitbucketReviewFetcher{
		client:  newBitbucketClient(baseURL, token),
		project: project,
		repo:    repo,
	}
}

type bbActivity struct {
	Action string `json:"action"`
	User   struct {
		Slug string `json:"slug"`
	} `json:"user"`
	CommentAnchor *struct{} `json:"commentAnchor"`
	Comment       *struct {
		Text string `json:"text"`
	} `json:"comment"`
}

type bbActivitiesResponse struct {
	Values []bbActivity `json:"values"`
}

// FetchLatestReview returns the latest review from a trusted reviewer via Bitbucket Server activities.
func (b *bitbucketReviewFetcher) FetchLatestReview(
	ctx context.Context,
	prURL string,
	allowedReviewers []string,
) (*ReviewResult, error) {
	prID, err := parseBitbucketPRID(prURL)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "extract PR ID")
	}

	var activities bbActivitiesResponse
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/activities",
		b.project, b.repo, prID)
	if err := b.client.do(ctx, "GET", path, nil, &activities); err != nil {
		return nil, errors.Wrap(ctx, err, "fetch PR activities")
	}

	allowed := make(map[string]bool, len(allowedReviewers))
	for _, r := range allowedReviewers {
		allowed[r] = true
	}

	var last *ReviewResult
	for _, act := range activities.Values {
		if !allowed[act.User.Slug] {
			continue
		}
		switch act.Action {
		case "APPROVED":
			last = &ReviewResult{Verdict: ReviewVerdictApproved}
		case "UNAPPROVED", "REVIEWED":
			commentBody := ""
			if act.Comment != nil {
				commentBody = act.Comment.Text
			}
			last = &ReviewResult{Verdict: ReviewVerdictChangesRequested, Body: commentBody}
		}
	}

	if last == nil {
		return &ReviewResult{Verdict: ReviewVerdictNone}, nil
	}
	return last, nil
}

// FetchPRState returns the Bitbucket Server PR state: "OPEN", "MERGED", or "CLOSED".
// Bitbucket's "DECLINED" is mapped to "CLOSED" for compatibility with the review poller.
func (b *bitbucketReviewFetcher) FetchPRState(ctx context.Context, prURL string) (string, error) {
	prID, err := parseBitbucketPRID(prURL)
	if err != nil {
		return "", errors.Wrap(ctx, err, "extract PR ID")
	}

	var pr struct {
		State string `json:"state"`
	}
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
		b.project, b.repo, prID)
	if err := b.client.do(ctx, "GET", path, nil, &pr); err != nil {
		return "", errors.Wrap(ctx, err, "fetch PR state")
	}

	// Normalize DECLINED to CLOSED to match GitHub conventions expected by the review poller.
	if pr.State == "DECLINED" {
		return "CLOSED", nil
	}
	return pr.State, nil
}
