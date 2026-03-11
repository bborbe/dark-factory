// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bborbe/errors"
)

// bitbucketReviewFetcher implements ReviewFetcher for Bitbucket Server.
type bitbucketReviewFetcher struct {
	baseURL string
	token   string
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
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
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
	prID, err := extractBitbucketPRID(prURL)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "extract PR ID")
	}

	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d/activities",
		b.baseURL, b.project, b.repo, prID,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "create activities request")
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "execute activities request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf(ctx, "activities returned status %d: %s", resp.StatusCode, body)
	}

	var activities bbActivitiesResponse
	if err := json.Unmarshal(body, &activities); err != nil {
		return nil, errors.Wrap(ctx, err, "parse activities response")
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

// FetchPRState returns the Bitbucket Server PR state: "OPEN", "MERGED", or "DECLINED".
func (b *bitbucketReviewFetcher) FetchPRState(ctx context.Context, prURL string) (string, error) {
	prID, err := extractBitbucketPRID(prURL)
	if err != nil {
		return "", errors.Wrap(ctx, err, "extract PR ID")
	}

	url := fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/pull-requests/%d",
		b.baseURL, b.project, b.repo, prID,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", errors.Wrap(ctx, err, "create PR state request")
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(ctx, err, "execute PR state request")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf(
			ctx,
			"fetch PR state returned status %d: %s",
			resp.StatusCode,
			body,
		)
	}

	var pr struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &pr); err != nil {
		return "", errors.Wrap(ctx, err, "parse PR state response")
	}

	// Normalize DECLINED to CLOSED to match GitHub conventions expected by the review poller.
	if pr.State == "DECLINED" {
		return "CLOSED", nil
	}
	return pr.State, nil
}
