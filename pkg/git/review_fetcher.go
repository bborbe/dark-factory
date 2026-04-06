// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// ReviewVerdict represents the outcome of a PR review.
type ReviewVerdict string

const (
	ReviewVerdictNone             ReviewVerdict = ""
	ReviewVerdictApproved         ReviewVerdict = "approved"
	ReviewVerdictChangesRequested ReviewVerdict = "changes_requested"
)

// ReviewResult holds the latest review from a trusted reviewer.
type ReviewResult struct {
	Verdict ReviewVerdict
	Body    string // full review body text
}

//counterfeiter:generate -o ../../mocks/review_fetcher.go --fake-name ReviewFetcher . ReviewFetcher

// ReviewFetcher polls a GitHub PR for reviews from trusted reviewers.
type ReviewFetcher interface {
	// FetchLatestReview returns the latest review from a trusted reviewer.
	// Returns ReviewVerdictNone if no trusted review exists yet.
	FetchLatestReview(
		ctx context.Context,
		prURL string,
		allowedReviewers []string,
	) (*ReviewResult, error)
	// FetchPRState returns the raw PR state string: "OPEN", "MERGED", "CLOSED".
	FetchPRState(ctx context.Context, prURL string) (string, error)
}

// NewReviewFetcher creates a new ReviewFetcher.
func NewReviewFetcher(ghToken string) ReviewFetcher {
	return &reviewFetcher{
		ghToken: ghToken,
	}
}

// reviewFetcher implements ReviewFetcher.
type reviewFetcher struct {
	ghToken string
}

// reviewEntry is one JSON object emitted by the jq filter.
type reviewEntry struct {
	State  string `json:"state"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

// FetchLatestReview returns the latest review from a trusted reviewer.
func (r *reviewFetcher) FetchLatestReview(
	ctx context.Context,
	prURL string,
	allowedReviewers []string,
) (*ReviewResult, error) {
	if err := ValidatePRURL(ctx, prURL); err != nil {
		return nil, errors.Wrap(ctx, err, "validate PR URL")
	}
	// #nosec G204 -- prURL validated by ValidatePRURL
	cmd := exec.CommandContext(
		ctx,
		"gh",
		"pr",
		"view",
		prURL,
		"--json",
		"reviews",
		"--jq",
		`.reviews[] | {state: .state, author: .author.login, body: .body}`,
	)
	if r.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+r.ghToken)
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "execute gh pr view reviews")
	}
	return parseReviews(ctx, output, allowedReviewers)
}

// FetchPRState returns the raw PR state: "OPEN", "MERGED", or "CLOSED".
func (r *reviewFetcher) FetchPRState(ctx context.Context, prURL string) (string, error) {
	if err := ValidatePRURL(ctx, prURL); err != nil {
		return "", errors.Wrap(ctx, err, "validate PR URL")
	}
	// #nosec G204 -- prURL validated by ValidatePRURL
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prURL, "--json", "state", "--jq", ".state")
	if r.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+r.ghToken)
	}
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "execute gh pr view state")
	}
	return strings.TrimSpace(string(output)), nil
}

// parseReviews parses NDJSON review output and returns the latest trusted review.
func parseReviews(
	ctx context.Context,
	output []byte,
	allowedReviewers []string,
) (*ReviewResult, error) {
	allowed := make(map[string]bool, len(allowedReviewers))
	for _, rev := range allowedReviewers {
		allowed[rev] = true
	}

	var last *ReviewResult
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry reviewEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, errors.Wrap(ctx, err, "parse review json")
		}
		if !allowed[entry.Author] {
			continue
		}
		verdict := ReviewVerdictNone
		switch entry.State {
		case "APPROVED":
			verdict = ReviewVerdictApproved
		case "CHANGES_REQUESTED":
			verdict = ReviewVerdictChangesRequested
		}
		last = &ReviewResult{Verdict: verdict, Body: entry.Body}
	}

	if last == nil {
		return &ReviewResult{Verdict: ReviewVerdictNone}, nil
	}
	return last, nil
}
