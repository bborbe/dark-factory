// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/bborbe/errors"
)

// bitbucketPRCreator implements PRCreator for Bitbucket Server.
type bitbucketPRCreator struct {
	client        *bitbucketClient
	project       string
	repo          string
	defaultBranch string
	reviewers     []string
}

// NewBitbucketPRCreator creates a PRCreator backed by the Bitbucket Server REST API.
func NewBitbucketPRCreator(
	baseURL string,
	token string,
	project string,
	repo string,
	defaultBranch string,
	reviewers []string,
) PRCreator {
	return &bitbucketPRCreator{
		client:        newBitbucketClient(baseURL, token),
		project:       project,
		repo:          repo,
		defaultBranch: defaultBranch,
		reviewers:     reviewers,
	}
}

type bbPRRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	FromRef     bbRef        `json:"fromRef"`
	ToRef       bbRef        `json:"toRef"`
	Reviewers   []bbReviewer `json:"reviewers"`
}

type bbRef struct {
	ID         string       `json:"id"`
	Repository bbRepository `json:"repository"`
}

type bbRepository struct {
	Slug    string    `json:"slug"`
	Project bbProject `json:"project"`
}

type bbProject struct {
	Key string `json:"key"`
}

type bbReviewer struct {
	User bbUser `json:"user"`
}

type bbUser struct {
	Name string `json:"name"`
}

type bbPRResponse struct {
	Links struct {
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
	ID int `json:"id"`
}

// Create creates a Bitbucket Server pull request and returns its URL.
func (b *bitbucketPRCreator) Create(
	ctx context.Context,
	title string,
	body string,
) (string, error) {
	currentBranch, err := currentGitBranch(ctx)
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current branch")
	}

	targetBranch := b.defaultBranch
	if targetBranch == "" {
		targetBranch = "master"
	}

	reviewers := make([]bbReviewer, 0, len(b.reviewers))
	for _, r := range b.reviewers {
		reviewers = append(reviewers, bbReviewer{User: bbUser{Name: r}})
	}

	repo := bbRepository{
		Slug:    b.repo,
		Project: bbProject{Key: b.project},
	}

	reqBody := bbPRRequest{
		Title:       title,
		Description: body,
		FromRef: bbRef{
			ID:         "refs/heads/" + currentBranch,
			Repository: repo,
		},
		ToRef: bbRef{
			ID:         "refs/heads/" + targetBranch,
			Repository: repo,
		},
		Reviewers: reviewers,
	}

	var prResp bbPRResponse
	path := fmt.Sprintf("/rest/api/1.0/projects/%s/repos/%s/pull-requests", b.project, b.repo)
	if err := b.client.do(ctx, "POST", path, reqBody, &prResp); err != nil {
		return "", errors.Wrap(ctx, err, "create pull request")
	}

	if len(prResp.Links.Self) > 0 {
		slog.Info("created Bitbucket PR", "id", prResp.ID, "url", prResp.Links.Self[0].Href)
		return prResp.Links.Self[0].Href, nil
	}
	prURL := fmt.Sprintf(
		"%s/projects/%s/repos/%s/pull-requests/%d",
		b.client.baseURL, b.project, b.repo, prResp.ID,
	)
	slog.Info("created Bitbucket PR", "id", prResp.ID, "url", prURL)
	return prURL, nil
}

// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
func (b *bitbucketPRCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
	branchRef := url.QueryEscape("refs/heads/" + branch)
	path := fmt.Sprintf(
		"/rest/api/1.0/projects/%s/repos/%s/pull-requests?state=OPEN&at=%s",
		b.project, b.repo, branchRef,
	)

	var result struct {
		Values []bbPRResponse `json:"values"`
	}
	if err := b.client.do(ctx, "GET", path, nil, &result); err != nil {
		return "", errors.Wrap(ctx, err, "find open pull request")
	}

	if len(result.Values) == 0 {
		return "", nil
	}

	pr := result.Values[0]
	if len(pr.Links.Self) > 0 {
		return pr.Links.Self[0].Href, nil
	}
	return fmt.Sprintf(
		"%s/projects/%s/repos/%s/pull-requests/%d",
		b.client.baseURL, b.project, b.repo, pr.ID,
	), nil
}

// currentGitBranch returns the current git branch name.
func currentGitBranch(ctx context.Context) (string, error) {
	b := NewBrancher()
	return b.CurrentBranch(ctx)
}
