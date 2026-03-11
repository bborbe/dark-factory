// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// bitbucketCollaboratorFetcher implements CollaboratorFetcher for Bitbucket Server.
// It fetches default reviewers from the Bitbucket Server default-reviewers plugin.
type bitbucketCollaboratorFetcher struct {
	baseURL       string
	token         string
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
		baseURL:       strings.TrimRight(baseURL, "/"),
		token:         token,
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
// Returns nil on error (best-effort).
func (b *bitbucketCollaboratorFetcher) Fetch(ctx context.Context) []string {
	targetBranch := b.defaultBranch
	if targetBranch == "" {
		targetBranch = "master"
	}

	url := fmt.Sprintf(
		"%s/rest/default-reviewers/1.0/projects/%s/repos/%s/reviewers?sourceRepoId=&targetRepoId=&sourceRefId=refs/heads/feature&targetRefId=refs/heads/%s",
		b.baseURL,
		b.project,
		b.repo,
		targetBranch,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		slog.Warn("bitbucket: failed to create default-reviewers request", "error", err)
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("bitbucket: default-reviewers request failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		slog.Warn("bitbucket: default-reviewers returned non-200", "status", resp.StatusCode)
		return nil
	}

	var result bbDefaultReviewersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("bitbucket: failed to parse default-reviewers response", "error", err)
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
